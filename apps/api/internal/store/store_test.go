package store

import "testing"

func TestNormalizeDatabaseURLStripsDriverSuffix(t *testing.T) {
	cases := map[string]string{
		"postgresql+asyncpg://u:p@h:5432/db": "postgres://u:p@h:5432/db",
		"postgresql+psycopg://u:p@h:5432/db": "postgres://u:p@h:5432/db",
		"postgresql://u:p@h:5432/db":         "postgres://u:p@h:5432/db",
		"postgres://u:p@h:5432/db":           "postgres://u:p@h:5432/db",
	}
	for input, want := range cases {
		if got := NormalizeDatabaseURL(input); got != want {
			t.Fatalf("NormalizeDatabaseURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSplitStatementsSplitsDDL(t *testing.T) {
	script := "CREATE TABLE a (id int); CREATE TABLE b (id int);\n"
	statements := splitStatements(script)
	if len(statements) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(statements))
	}
	if statements[0] != "CREATE TABLE a (id int)" {
		t.Fatalf("unexpected first statement %q", statements[0])
	}
}
