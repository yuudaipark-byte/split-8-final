package main

import (
	"database/sql"
)

// ParcelStore хранит и изменяет сведения о посылках в SQLite.
type ParcelStore struct {
	db *sql.DB
}

func NewParcelStore(db *sql.DB) ParcelStore {
	return ParcelStore{db: db}
}

// Add создаёт новую посылку и возвращает её трек-номер (ID в таблице).
func (s ParcelStore) Add(p Parcel) (int, error) {
	result, err := s.db.Exec(
		`INSERT INTO parcel (client, status, address, created_at)
		 VALUES (?, ?, ?, ?)`,
		p.Client, p.Status, p.Address, p.CreatedAt,
	)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// Get возвращает одну посылку по трек-номеру.
// Для отсутствующей посылки возвращает sql.ErrNoRows.
func (s ParcelStore) Get(number int) (Parcel, error) {
	var p Parcel
	err := s.db.QueryRow(
		`SELECT number, client, status, address, created_at
		 FROM parcel WHERE number = ?`, number,
	).Scan(&p.Number, &p.Client, &p.Status, &p.Address, &p.CreatedAt)
	if err != nil {
		return Parcel{}, err
	}
	return p, nil
}

// GetByClient возвращает все посылки клиента в порядке трек-номеров.
func (s ParcelStore) GetByClient(client int) ([]Parcel, error) {
	rows, err := s.db.Query(
		`SELECT number, client, status, address, created_at
		 FROM parcel WHERE client = ? ORDER BY number`, client,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	parcels := make([]Parcel, 0)
	for rows.Next() {
		var p Parcel
		if err := rows.Scan(&p.Number, &p.Client, &p.Status, &p.Address, &p.CreatedAt); err != nil {
			return nil, err
		}
		parcels = append(parcels, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return parcels, nil
}

// SetStatus меняет статус указанной посылки.
func (s ParcelStore) SetStatus(number int, status string) error {
	_, err := s.db.Exec(
		`UPDATE parcel SET status = ? WHERE number = ?`, status, number,
	)
	return err
}

// SetAddress меняет адрес только у посылки в статусе registered.
func (s ParcelStore) SetAddress(number int, address string) error {
	_, err := s.db.Exec(
		`UPDATE parcel SET address = ? WHERE number = ? AND status = ?`,
		address, number, ParcelStatusRegistered,
	)
	return err
}

// Delete удаляет посылку только в статусе registered.
func (s ParcelStore) Delete(number int) error {
	_, err := s.db.Exec(
		`DELETE FROM parcel WHERE number = ? AND status = ?`,
		number, ParcelStatusRegistered,
	)
	return err
}
