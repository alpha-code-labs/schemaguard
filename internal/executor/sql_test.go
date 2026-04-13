package executor

import (
	"reflect"
	"testing"
)

func TestSplitStatementsEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\n\t", ";;;"} {
		if got := SplitStatements(in); len(got) != 0 {
			t.Errorf("SplitStatements(%q) = %v, want empty", in, got)
		}
	}
}

func TestSplitStatementsSimple(t *testing.T) {
	in := "SELECT 1; SELECT 2;SELECT 3"
	want := []string{"SELECT 1", "SELECT 2", "SELECT 3"}
	got := SplitStatements(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSplitStatementsTrailingSemicolonOptional(t *testing.T) {
	in := "CREATE TABLE t (id int)"
	got := SplitStatements(in)
	if len(got) != 1 || got[0] != "CREATE TABLE t (id int)" {
		t.Errorf("got %q, want single statement", got)
	}
}

func TestSplitStatementsLineComment(t *testing.T) {
	in := "-- a comment; still a comment\nSELECT 1;\nSELECT 2; -- trailing\n"
	got := SplitStatements(in)
	if len(got) != 2 {
		t.Fatalf("got %d statements, want 2: %q", len(got), got)
	}
	if got[1] != "SELECT 2" {
		t.Errorf("got[1] = %q, want SELECT 2", got[1])
	}
}

func TestSplitStatementsBlockComment(t *testing.T) {
	in := "/* block ; with semicolon */ SELECT 1;\n" +
		"/* nested /* block */ still comment */ SELECT 2;"
	got := SplitStatements(in)
	if len(got) != 2 {
		t.Fatalf("got %d statements, want 2: %q", len(got), got)
	}
}

func TestSplitStatementsSingleQuotedSemicolon(t *testing.T) {
	in := "INSERT INTO t VALUES ('hello; world'); SELECT 1;"
	got := SplitStatements(in)
	if len(got) != 2 {
		t.Fatalf("got %d statements, want 2: %q", len(got), got)
	}
	if got[0] != "INSERT INTO t VALUES ('hello; world')" {
		t.Errorf("got[0] = %q", got[0])
	}
}

func TestSplitStatementsEscapedQuote(t *testing.T) {
	in := "INSERT INTO t VALUES ('it''s ok; really'); SELECT 1;"
	got := SplitStatements(in)
	if len(got) != 2 {
		t.Fatalf("got %d statements, want 2: %q", len(got), got)
	}
	if got[0] != "INSERT INTO t VALUES ('it''s ok; really')" {
		t.Errorf("got[0] = %q", got[0])
	}
}

func TestSplitStatementsDoubleQuotedIdentifier(t *testing.T) {
	in := `CREATE TABLE "weird;name" (id int); SELECT 1;`
	got := SplitStatements(in)
	if len(got) != 2 {
		t.Fatalf("got %d statements, want 2: %q", len(got), got)
	}
}

func TestSplitStatementsDollarQuoted(t *testing.T) {
	in := `CREATE FUNCTION f() RETURNS int AS $$
BEGIN
  RETURN 1;
END;
$$ LANGUAGE plpgsql;
SELECT 1;`
	got := SplitStatements(in)
	if len(got) != 2 {
		t.Fatalf("got %d statements, want 2: %q", len(got), got)
	}
	if got[1] != "SELECT 1" {
		t.Errorf("got[1] = %q, want SELECT 1", got[1])
	}
}

func TestSplitStatementsDollarQuotedWithTag(t *testing.T) {
	in := `CREATE FUNCTION f() RETURNS text AS $body$
  SELECT 'hello; world';
$body$ LANGUAGE sql;
SELECT 1;`
	got := SplitStatements(in)
	if len(got) != 2 {
		t.Fatalf("got %d statements, want 2: %q", len(got), got)
	}
}

func TestSplitStatementsDiscardsEmptyStatements(t *testing.T) {
	in := "SELECT 1;;\n;\nSELECT 2;"
	got := SplitStatements(in)
	if len(got) != 2 || got[0] != "SELECT 1" || got[1] != "SELECT 2" {
		t.Errorf("got %q", got)
	}
}

func TestHasExplicitTransactionPositive(t *testing.T) {
	cases := [][]string{
		{"BEGIN", "CREATE TABLE t (id int)", "COMMIT"},
		{"START TRANSACTION", "SELECT 1", "COMMIT"},
		{"SELECT 1", "COMMIT"},
		{"SELECT 1", "ROLLBACK"},
		{"SELECT 1", "END"},
	}
	for _, stmts := range cases {
		if !HasExplicitTransaction(stmts) {
			t.Errorf("HasExplicitTransaction(%v) = false, want true", stmts)
		}
	}
}

func TestHasExplicitTransactionNegative(t *testing.T) {
	cases := [][]string{
		{"CREATE TABLE t (id int)"},
		{"CREATE TABLE t (id int)", "INSERT INTO t VALUES (1)"},
		{"SELECT 1"},
		{},
	}
	for _, stmts := range cases {
		if HasExplicitTransaction(stmts) {
			t.Errorf("HasExplicitTransaction(%v) = true, want false", stmts)
		}
	}
}

func TestHasExplicitTransactionIgnoresLeadingComments(t *testing.T) {
	stmts := []string{
		"-- comment\n-- more comment\nBEGIN",
		"SELECT 1",
		"COMMIT",
	}
	if !HasExplicitTransaction(stmts) {
		t.Errorf("HasExplicitTransaction with leading comments failed to detect BEGIN")
	}
}

func TestIsTxControlCaseInsensitive(t *testing.T) {
	cases := []string{"begin", "Begin", "BEGIN", "commit", "ROLLBACK"}
	for _, s := range cases {
		if !isTxControl(s) {
			t.Errorf("isTxControl(%q) = false, want true", s)
		}
	}
}

func TestTrimLeadingCommentsAndSpaceBlockComment(t *testing.T) {
	in := "/* hi */ /* there */  BEGIN"
	got := trimLeadingCommentsAndSpace(in)
	if got != "BEGIN" {
		t.Errorf("got %q, want BEGIN", got)
	}
}
