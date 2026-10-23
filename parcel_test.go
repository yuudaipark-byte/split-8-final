package main

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newTestStore создаёт отдельную SQLite-базу для каждого теста.
// Исходный файл tracker.db при выполнении тестов не изменяется.
func newTestStore(t *testing.T) ParcelStore {
	t.Helper()

	path := filepath.Join(t.TempDir(), "tracker_test.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	_, err = db.Exec(`
		CREATE TABLE parcel (
			number INTEGER PRIMARY KEY AUTOINCREMENT,
			client INTEGER NOT NULL,
			status VARCHAR(128) NOT NULL,
			address VARCHAR(512) NOT NULL,
			created_at TEXT NOT NULL
		)`)
	require.NoError(t, err)
	return NewParcelStore(db)
}

func getTestParcel() Parcel {
	return Parcel{
		Client:    1000,
		Status:    ParcelStatusRegistered,
		Address:   "test",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// TestAddGetDelete проверяет регистрацию, получение и удаление посылки.
func TestAddGetDelete(t *testing.T) {
	store := newTestStore(t)
	parcel := getTestParcel()

	id, err := store.Add(parcel)
	require.NoError(t, err)
	require.Positive(t, id)
	parcel.Number = id

	stored, err := store.Get(id)
	require.NoError(t, err)
	require.Equal(t, parcel, stored)

	require.NoError(t, store.Delete(id))
	_, err = store.Get(id)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

// TestSetAddress проверяет изменение адреса только у зарегистрированной посылки.
func TestSetAddress(t *testing.T) {
	store := newTestStore(t)
	parcel := getTestParcel()
	id, err := store.Add(parcel)
	require.NoError(t, err)

	newAddress := "new test address"
	require.NoError(t, store.SetAddress(id, newAddress))
	stored, err := store.Get(id)
	require.NoError(t, err)
	require.Equal(t, newAddress, stored.Address)

	// После отправки адрес менять нельзя.
	require.NoError(t, store.SetStatus(id, ParcelStatusSent))
	require.NoError(t, store.SetAddress(id, "запрещённый адрес"))
	stored, err = store.Get(id)
	require.NoError(t, err)
	require.Equal(t, newAddress, stored.Address)
}

// TestSetStatus проверяет обновление статуса.
func TestSetStatus(t *testing.T) {
	store := newTestStore(t)
	id, err := store.Add(getTestParcel())
	require.NoError(t, err)

	require.NoError(t, store.SetStatus(id, ParcelStatusSent))
	stored, err := store.Get(id)
	require.NoError(t, err)
	require.Equal(t, ParcelStatusSent, stored.Status)

	require.NoError(t, store.SetStatus(id, ParcelStatusDelivered))
	stored, err = store.Get(id)
	require.NoError(t, err)
	require.Equal(t, ParcelStatusDelivered, stored.Status)
}

// TestGetByClient проверяет, что возвращаются все и только посылки указанного клиента.
func TestGetByClient(t *testing.T) {
	store := newTestStore(t)
	const client = 12345

	want := make(map[int]Parcel)
	for i := 0; i < 3; i++ {
		parcel := getTestParcel()
		parcel.Client = client
		parcel.Address = "адрес " + string(rune('A'+i))
		id, err := store.Add(parcel)
		require.NoError(t, err)
		parcel.Number = id
		want[id] = parcel
	}

	// Посылка другого клиента не должна попасть в результат.
	other := getTestParcel()
	other.Client = 54321
	_, err := store.Add(other)
	require.NoError(t, err)

	stored, err := store.GetByClient(client)
	require.NoError(t, err)
	require.Len(t, stored, len(want))
	for _, parcel := range stored {
		expected, ok := want[parcel.Number]
		require.True(t, ok, "неожиданная посылка № %d", parcel.Number)
		require.Equal(t, expected, parcel)
	}

	empty, err := store.GetByClient(999999)
	require.NoError(t, err)
	require.Empty(t, empty)
}

// TestDeleteSentParcel проверяет, что отправленную посылку нельзя удалить.
func TestDeleteSentParcel(t *testing.T) {
	store := newTestStore(t)
	id, err := store.Add(getTestParcel())
	require.NoError(t, err)

	require.NoError(t, store.SetStatus(id, ParcelStatusSent))
	require.NoError(t, store.Delete(id))
	stored, err := store.Get(id)
	require.NoError(t, err)
	require.Equal(t, ParcelStatusSent, stored.Status)
}

// TestParcelServiceLifecycle проверяет полный сценарий задания через ParcelService.
func TestParcelServiceLifecycle(t *testing.T) {
	store := newTestStore(t)
	service := NewParcelService(store)

	parcel, err := service.Register(7, "старый адрес")
	require.NoError(t, err)
	require.Positive(t, parcel.Number)
	require.Equal(t, ParcelStatusRegistered, parcel.Status)

	require.NoError(t, service.ChangeAddress(parcel.Number, "новый адрес"))
	stored, err := store.Get(parcel.Number)
	require.NoError(t, err)
	require.Equal(t, "новый адрес", stored.Address)

	require.NoError(t, service.NextStatus(parcel.Number))
	stored, err = store.Get(parcel.Number)
	require.NoError(t, err)
	require.Equal(t, ParcelStatusSent, stored.Status)

	// Отправленную посылку нельзя удалить или переадресовать.
	require.NoError(t, service.Delete(parcel.Number))
	require.NoError(t, service.ChangeAddress(parcel.Number, "ещё один адрес"))
	stored, err = store.Get(parcel.Number)
	require.NoError(t, err)
	require.Equal(t, "новый адрес", stored.Address)

	require.NoError(t, service.NextStatus(parcel.Number))
	require.NoError(t, service.NextStatus(parcel.Number)) // Доставленная остаётся доставленной.
	stored, err = store.Get(parcel.Number)
	require.NoError(t, err)
	require.Equal(t, ParcelStatusDelivered, stored.Status)

	fresh, err := service.Register(7, "удаляемая посылка")
	require.NoError(t, err)
	require.NoError(t, service.Delete(fresh.Number))
	_, err = store.Get(fresh.Number)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
