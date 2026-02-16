package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func New(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	conn, err := sql.Open("sqlite3", path+"?_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate: %w", err)
	}

	return db, nil
}

func (db *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS cost_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			subscription_id TEXT NOT NULL,
			resource_group TEXT,
			service_name TEXT NOT NULL,
			cost REAL NOT NULL,
			currency TEXT DEFAULT 'USD',
			date TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS alerts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			threshold REAL NOT NULL,
			subscription_id TEXT NOT NULL,
			enabled INTEGER DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_date ON cost_records(date)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_subscription ON cost_records(subscription_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_service ON cost_records(service_name)`,
	}

	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) GetConfig(key string) (string, error) {
	var value string
	err := db.conn.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (db *DB) SetConfig(key, value string) error {
	_, err := db.conn.Exec(`
		INSERT INTO config (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return err
}

type CostRecord struct {
	ID              int64
	SubscriptionID  string
	ResourceGroup   string
	ServiceName     string
	Cost            float64
	Currency        string
	Date            string
}

func (db *DB) SaveCostRecord(record CostRecord) error {
	_, err := db.conn.Exec(`
		INSERT INTO cost_records (subscription_id, resource_group, service_name, cost, currency, date)
		VALUES (?, ?, ?, ?, ?, ?)
	`, record.SubscriptionID, record.ResourceGroup, record.ServiceName, record.Cost, record.Currency, record.Date)
	return err
}

func (db *DB) SaveCostRecords(records []CostRecord) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO cost_records (subscription_id, resource_group, service_name, cost, currency, date)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		if _, err := stmt.Exec(r.SubscriptionID, r.ResourceGroup, r.ServiceName, r.Cost, r.Currency, r.Date); err != nil {
			return err
		}
	}

	return tx.Commit()
}

type CostFilter struct {
	StartDate   string
	EndDate     string
	ServiceName string
	GroupBy     string
}

func (db *DB) GetCostRecords(filter CostFilter) ([]CostRecord, error) {
	query := "SELECT id, subscription_id, resource_group, service_name, cost, currency, date FROM cost_records WHERE 1=1"
	args := []interface{}{}

	if filter.StartDate != "" {
		query += " AND date >= ?"
		args = append(args, filter.StartDate)
	}
	if filter.EndDate != "" {
		query += " AND date <= ?"
		args = append(args, filter.EndDate)
	}
	if filter.ServiceName != "" {
		query += " AND service_name = ?"
		args = append(args, filter.ServiceName)
	}

	query += " ORDER BY date DESC"

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CostRecord
	for rows.Next() {
		var r CostRecord
		if err := rows.Scan(&r.ID, &r.SubscriptionID, &r.ResourceGroup, &r.ServiceName, &r.Cost, &r.Currency, &r.Date); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func (db *DB) GetAggregatedCosts(filter CostFilter) (map[string]float64, error) {
	groupBy := "service_name"
	if filter.GroupBy == "ResourceGroup" {
		groupBy = "resource_group"
	}

	query := fmt.Sprintf("SELECT %s, SUM(cost) as total FROM cost_records WHERE 1=1", groupBy)
	args := []interface{}{}

	if filter.StartDate != "" {
		query += " AND date >= ?"
		args = append(args, filter.StartDate)
	}
	if filter.EndDate != "" {
		query += " AND date <= ?"
		args = append(args, filter.EndDate)
	}

	query += " GROUP BY " + groupBy

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var name string
		var total float64
		if err := rows.Scan(&name, &total); err != nil {
			return nil, err
		}
		result[name] = total
	}
	return result, nil
}
