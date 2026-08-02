package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/nnn/sanctions-service/internal/config"
)

func Connect(cfg *config.Config) (*sql.DB, error) {
	return openMySQL(cfg.DSN(), cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
}

// ConnectForSeeder uses a single long-lived connection suitable for multi-minute transactions.
func ConnectForSeeder(cfg *config.Config) (*sql.DB, error) {
	return openMySQL(cfg.SeederDSN(), 1, 1, 0)
}

func openMySQL(dsn string, maxOpen, maxIdle int, connMaxLifetime time.Duration) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	if connMaxLifetime > 0 {
		db.SetConnMaxLifetime(connMaxLifetime)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}
