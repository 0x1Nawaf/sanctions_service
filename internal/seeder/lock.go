package seeder

import (
	"database/sql"
	"fmt"
	"log"
)

const seederLockName = "sanctions_seeder"

func (s *Seeder) acquireSeederLock() error {
	var got sql.NullInt64
	if err := s.db.QueryRow(`SELECT GET_LOCK(?, 0)`, seederLockName).Scan(&got); err != nil {
		return fmt.Errorf("seeder lock: %w", err)
	}
	if !got.Valid || got.Int64 != 1 {
		return fmt.Errorf("another sanctions seeder is already running (could not acquire lock %q)", seederLockName)
	}
	log.Printf("Acquired seeder lock")
	return nil
}

func (s *Seeder) releaseSeederLock() {
	if _, err := s.db.Exec(`SELECT RELEASE_LOCK(?)`, seederLockName); err != nil {
		log.Printf("WARN: release seeder lock: %v", err)
	}
}
