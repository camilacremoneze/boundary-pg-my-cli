package boundary

import (
	"testing"
)

// parseDB and parseDBType are unexported — tested here as white-box (same package).

func TestParseDB(t *testing.T) {
	tests := []struct {
		desc string
		want string
	}{
		// Standard format used in Boundary target descriptions.
		{"type: postgres, name: myservice, port: 5432, db: mydb", "mydb"},
		{"type: mysql, db: orders", "orders"},
		{"type: mariadb, db: users_db", "users_db"},

		// Case-insensitive key.
		{"DB: analytics", "analytics"},
		{"Type: postgres, DB: reporting", "reporting"},

		// db: value with no trailing comma.
		{"db: warehouse", "warehouse"},

		// db: value followed by comma.
		{"db: core, extra: stuff", "core"},

		// No db field – returns empty string.
		{"type: postgres, name: nodb-service", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := parseDB(tt.desc)
		if got != tt.want {
			t.Errorf("parseDB(%q) = %q, want %q", tt.desc, got, tt.want)
		}
	}
}

func TestParseDBType(t *testing.T) {
	tests := []struct {
		desc string
		want string
	}{
		// Explicit postgres.
		{"type: postgres, db: mydb", "postgres"},
		{"TYPE: POSTGRES, db: x", "postgres"},

		// MySQL.
		{"type: mysql, db: orders", "mysql"},
		{"TYPE: MySQL, db: orders", "mysql"},

		// MariaDB.
		{"type: mariadb, db: users", "mariadb"},
		{"Type: MariaDB, db: users", "mariadb"},

		// Unknown type defaults to postgres.
		{"type: cockroachdb, db: x", "postgres"},
		{"type: mssql, db: x", "postgres"},

		// No type field – defaults to postgres.
		{"name: my-service, db: mydb", "postgres"},
		{"", "postgres"},
	}

	for _, tt := range tests {
		got := parseDBType(tt.desc)
		if got != tt.want {
			t.Errorf("parseDBType(%q) = %q, want %q", tt.desc, got, tt.want)
		}
	}
}
