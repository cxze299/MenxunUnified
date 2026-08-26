package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

func TestIsIgnorableMigrationError(t *testing.T) {
	for _, number := range []uint16{1060, 1061} {
		if !isIgnorableMigrationError(&mysqlDriver.MySQLError{Number: number}) {
			t.Fatalf("expected MySQL error %d to be retry-safe", number)
		}
	}
	if isIgnorableMigrationError(&mysqlDriver.MySQLError{Number: 1045}) {
		t.Fatal("access denied must not be ignored")
	}
	if isIgnorableMigrationError(errors.New("ordinary error")) {
		t.Fatal("ordinary errors must not be ignored")
	}
}

func TestPlatformCompletionMigrationSplitsIntoRunnableStatements(t *testing.T) {
	data, err := os.ReadFile("../../migrations/009_platform_completion.sql")
	if err != nil {
		t.Fatal(err)
	}
	statements := splitSQL(string(data))
	if len(statements) != 8 {
		t.Fatalf("migration split into %d statements, want 8", len(statements))
	}
	joined := strings.Join(statements, "\n")
	for _, required := range []string{"terms_accepted_at", "CREATE TABLE IF NOT EXISTS reflections", "migration_batches", "migration_source_records", "idx_assets_checksum_size"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("migration is missing %q", required)
		}
	}
}
