package executor

import (
	"strings"
)

// SplitStatements splits a PostgreSQL script into individual statements.
// It is a small, purpose-built lexer — not a full SQL parser — that
// correctly handles the constructs that actually appear in real
// migration files and would break a naive "split on semicolon" approach:
//
//   - single-line comments   (-- ...)
//   - block comments         (/* ... */, including nested block comments)
//   - single-quoted strings  ('...' with '' as the embedded-quote escape)
//   - double-quoted identifiers ("..." with "" escape)
//   - dollar-quoted strings  ($$...$$ and $tag$...$tag$, common in functions)
//
// Semicolons inside any of the above are preserved as part of the
// current statement; only top-level semicolons terminate a statement.
// Empty statements are discarded. Returned statements have leading and
// trailing whitespace trimmed but are otherwise unchanged.
func SplitStatements(sql string) []string {
	var out []string
	var cur strings.Builder

	flush := func() {
		stmt := strings.TrimSpace(cur.String())
		// Drop statements that are only whitespace or only comments.
		// trimLeadingCommentsAndSpace collapses leading comments; if
		// nothing executable remains, the "statement" is a no-op we
		// should not emit to the executor.
		if stmt != "" && trimLeadingCommentsAndSpace(stmt) != "" {
			out = append(out, stmt)
		}
		cur.Reset()
	}

	n := len(sql)
	i := 0
	for i < n {
		c := sql[i]

		// -- line comment
		if c == '-' && i+1 < n && sql[i+1] == '-' {
			for i < n && sql[i] != '\n' {
				cur.WriteByte(sql[i])
				i++
			}
			continue
		}

		// /* block comment */ with nesting
		if c == '/' && i+1 < n && sql[i+1] == '*' {
			depth := 1
			cur.WriteByte(sql[i])
			cur.WriteByte(sql[i+1])
			i += 2
			for i < n && depth > 0 {
				if i+1 < n && sql[i] == '/' && sql[i+1] == '*' {
					depth++
					cur.WriteByte(sql[i])
					cur.WriteByte(sql[i+1])
					i += 2
					continue
				}
				if i+1 < n && sql[i] == '*' && sql[i+1] == '/' {
					depth--
					cur.WriteByte(sql[i])
					cur.WriteByte(sql[i+1])
					i += 2
					continue
				}
				cur.WriteByte(sql[i])
				i++
			}
			continue
		}

		// 'single-quoted string' with '' escape
		if c == '\'' {
			cur.WriteByte(sql[i])
			i++
			for i < n {
				if sql[i] == '\'' {
					if i+1 < n && sql[i+1] == '\'' {
						cur.WriteByte(sql[i])
						cur.WriteByte(sql[i+1])
						i += 2
						continue
					}
					cur.WriteByte(sql[i])
					i++
					break
				}
				cur.WriteByte(sql[i])
				i++
			}
			continue
		}

		// "double-quoted identifier" with "" escape
		if c == '"' {
			cur.WriteByte(sql[i])
			i++
			for i < n {
				if sql[i] == '"' {
					if i+1 < n && sql[i+1] == '"' {
						cur.WriteByte(sql[i])
						cur.WriteByte(sql[i+1])
						i += 2
						continue
					}
					cur.WriteByte(sql[i])
					i++
					break
				}
				cur.WriteByte(sql[i])
				i++
			}
			continue
		}

		// $tag$ ... $tag$ dollar-quoted string (tag may be empty)
		if c == '$' {
			if tag, ok := readDollarTag(sql, i); ok {
				cur.WriteString(tag)
				i += len(tag)
				// Find the matching closing tag.
				end := strings.Index(sql[i:], tag)
				if end < 0 {
					// Unterminated: absorb the rest of the input and stop.
					cur.WriteString(sql[i:])
					i = n
					continue
				}
				cur.WriteString(sql[i : i+end])
				cur.WriteString(tag)
				i += end + len(tag)
				continue
			}
			// Not a dollar tag, fall through to normal byte handling.
		}

		// Top-level statement terminator.
		if c == ';' {
			flush()
			i++
			continue
		}

		cur.WriteByte(sql[i])
		i++
	}

	// Trailing statement with no terminator.
	flush()
	return out
}

// readDollarTag recognizes a dollar-quoted opening tag at sql[i] and
// returns the full tag text ($...$) and true if valid. It returns
// ("", false) when sql[i] is a lone '$' that is not part of a tag.
//
// Postgres dollar tags are: $$ or $tag$ where tag is a C-style identifier
// (letters, digits, underscores; may not start with a digit).
func readDollarTag(sql string, i int) (string, bool) {
	if i >= len(sql) || sql[i] != '$' {
		return "", false
	}
	j := i + 1
	// Tag body is zero or more identifier characters.
	if j < len(sql) && isDigitByte(sql[j]) {
		// Tag cannot start with a digit.
		return "", false
	}
	for j < len(sql) && isIdentByte(sql[j]) {
		j++
	}
	if j < len(sql) && sql[j] == '$' {
		return sql[i : j+1], true
	}
	return "", false
}

func isIdentByte(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_'
}

func isDigitByte(c byte) bool {
	return c >= '0' && c <= '9'
}

// HasExplicitTransaction reports whether any statement in stmts is a
// transaction-control statement (BEGIN, START TRANSACTION, COMMIT,
// ROLLBACK, or END). When true, the executor MUST NOT wrap the migration
// in its own transaction — the file is already directing its own
// transaction boundaries and wrapping would produce nested-transaction
// errors.
//
// The check uses the first SQL keyword of each statement after leading
// comments and whitespace have been stripped by SplitStatements. It is
// intentionally simple: the v1 rule is "any top-level tx-control
// statement makes the whole migration self-managed".
func HasExplicitTransaction(stmts []string) bool {
	for _, s := range stmts {
		if isTxControl(s) {
			return true
		}
	}
	return false
}

func isTxControl(stmt string) bool {
	// Skip leading -- and /* */ comments so statements that begin with a
	// leading comment still classify correctly.
	stmt = trimLeadingCommentsAndSpace(stmt)
	if stmt == "" {
		return false
	}
	// Take the first whitespace-delimited token, uppercase it.
	fields := strings.Fields(stmt)
	if len(fields) == 0 {
		return false
	}
	first := strings.ToUpper(strings.TrimRight(fields[0], ";"))
	switch first {
	case "BEGIN", "COMMIT", "ROLLBACK", "END":
		return true
	case "START":
		if len(fields) >= 2 && strings.ToUpper(fields[1]) == "TRANSACTION" {
			return true
		}
	}
	return false
}

func trimLeadingCommentsAndSpace(s string) string {
	for {
		s = strings.TrimLeft(s, " \t\r\n")
		if strings.HasPrefix(s, "--") {
			if idx := strings.IndexByte(s, '\n'); idx >= 0 {
				s = s[idx+1:]
				continue
			}
			return ""
		}
		if strings.HasPrefix(s, "/*") {
			if idx := strings.Index(s, "*/"); idx >= 0 {
				s = s[idx+2:]
				continue
			}
			return ""
		}
		return s
	}
}
