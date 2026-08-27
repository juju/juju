// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbrepl

import (
	"context"
	"database/sql"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/chzyer/readline"

	"github.com/juju/juju/core/database"
)

// compile-time check that sqlCompleter satisfies readline.AutoCompleter.
var _ readline.AutoCompleter = (*sqlCompleter)(nil)

// sqlCompleter implements readline.AutoCompleter to provide tab-completion for
// SELECT statements in the DB REPL. It completes SQL keywords, table names
// (after FROM/JOIN) and column names (in the SELECT list or after a
// "table." prefix), querying the currently active database for schema.
type sqlCompleter struct {
	// db returns the database to introspect at completion time. The REPL can
	// switch databases via .switch/.open, so the schema must be queried lazily
	// rather than cached.
	db func() database.TxnRunner

	ctx context.Context
}

func newSQLCompleter(db func() database.TxnRunner, ctx context.Context) *sqlCompleter {
	return &sqlCompleter{
		db:  db,
		ctx: ctx,
	}
}

// Do implements readline.AutoCompleter. It returns the set of candidates
// (as suffixes appended after the current word) and the offset into the line
// where those candidates start.
func (c *sqlCompleter) Do(line []rune, pos int) ([][]rune, int) {
	if pos > len(line) {
		pos = len(line)
	}
	text := string(line[:pos])
	tok := currentToken(text)

	if strings.Contains(tok, ".") {
		return c.dotDo(tok)
	}

	return filterSuffix(c.candidates(text, tok), tok)
}

// dotDo completes "table.col" style tokens, returning column names of the
// referenced table as suffixes after the current word.
func (c *sqlCompleter) dotDo(tok string) ([][]rune, int) {
	table, col := splitDotToken(tok)
	t := c.resolveTable(table)
	if t == "" {
		return nil, 0
	}

	out := make([][]rune, 0, 4)
	rc := []rune(col)
	for _, name := range c.columns(t) {
		if !hasPrefixCI(name, col) {
			continue
		}
		out = append(out, []rune(name)[len(rc):])
	}
	if len(out) == 0 {
		return nil, 0
	}
	return out, utf8.RuneCountInString(tok)
}

// candidates determines what to complete based on the SQL text typed so far.
func (c *sqlCompleter) candidates(text, tok string) []string {
	upper := strings.ToUpper(text)
	words := strings.Fields(upper)
	curWordIdx := len(words) - 1

	selectIdx := indexOf(words, "SELECT")
	fromIdx := lastIndexOf(words, "FROM")
	joinIdx := lastIndexOf(words, "JOIN")
	tabIdx := max(fromIdx, joinIdx)

	// Table list region: cursor at or after FROM/JOIN and before any clause
	// keyword terminates the table list.
	if tabIdx != -1 && curWordIdx >= tabIdx && firstIndexFrom(words, tabIdx+1, curWordIdx+1, clauseKeywords) == -1 {
		return c.tables()
	}

	// Column region: after SELECT, before the table list.
	if selectIdx != -1 && curWordIdx > selectIdx {
		return c.columnsPlusKeywords(words)
	}

	// Default: statement start, complete keywords.
	return keywords
}

// columnsPlusKeywords completes the columns of the table referenced in the
// FROM/JOIN clauses (if any) plus SQL keywords.
func (c *sqlCompleter) columnsPlusKeywords(words []string) []string {
	fromIdx := lastIndexOf(words, "FROM")
	joinIdx := lastIndexOf(words, "JOIN")
	tabIdx := max(fromIdx, joinIdx)
	if tabIdx != -1 && tabIdx+1 < len(words) {
		if t := c.resolveTable(words[tabIdx+1]); t != "" {
			return append(c.columns(t), keywords...)
		}
	}
	return keywords
}

// tables returns the names of all tables in the current database.
func (c *sqlCompleter) tables() []string {
	db := c.db()
	if db == nil {
		return nil
	}
	var out []string
	_ = db.StdTxn(c.ctx, func(ctx context.Context, tx *sql.Tx) error {
		out = nil
		rows, err := tx.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			out = append(out, name)
		}
		return rows.Err()
	})
	return out
}

// columns returns the column names of the given table.
func (c *sqlCompleter) columns(table string) []string {
	db := c.db()
	if db == nil {
		return nil
	}
	var out []string
	_ = db.StdTxn(c.ctx, func(ctx context.Context, tx *sql.Tx) error {
		out = nil
		rows, err := tx.QueryContext(ctx, "SELECT name FROM pragma_table_info(?) ORDER BY cid", table)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			out = append(out, name)
		}
		return rows.Err()
	})
	return out
}

// resolveTable matches the given (possibly partial) table name case-insensitively
// against the actual schema, returning the canonical name, or "" if none match.
func (c *sqlCompleter) resolveTable(name string) string {
	if name == "" {
		return ""
	}
	for _, t := range c.tables() {
		if strings.EqualFold(t, name) {
			return t
		}
	}
	return ""
}

// currentToken returns the word being typed, i.e. the text after the last
// whitespace.
func currentToken(text string) string {
	i := strings.LastIndexAny(text, " \t")
	if i == -1 {
		return text
	}
	return text[i+1:]
}

// splitDotToken splits a "table.column" token into its table and column parts.
func splitDotToken(tok string) (string, string) {
	i := strings.LastIndex(tok, ".")
	if i == -1 {
		return "", tok
	}
	return tok[:i], tok[i+1:]
}

// filterSuffix filters candidates by case-insensitive prefix and returns them
// as suffixes after the current word, along with the length of that word.
func filterSuffix(cands []string, tok string) ([][]rune, int) {
	rt := []rune(tok)
	out := make([][]rune, 0, len(cands))
	for _, cand := range cands {
		if !hasPrefixCI(cand, tok) {
			continue
		}
		out = append(out, []rune(cand)[len(rt):])
	}
	if len(out) == 0 {
		return nil, 0
	}
	return out, len(rt)
}

// hasPrefixCI reports whether name has the given case-insensitive prefix.
func hasPrefixCI(name, prefix string) bool {
	n := []rune(name)
	p := []rune(prefix)
	if len(p) == 0 {
		return true
	}
	if len(p) > len(n) {
		return false
	}
	for i := range p {
		if unicode.ToLower(n[i]) != unicode.ToLower(p[i]) {
			return false
		}
	}
	return true
}

func indexOf(words []string, target string) int {
	for i, w := range words {
		if w == target {
			return i
		}
	}
	return -1
}

func lastIndexOf(words []string, target string) int {
	for i := len(words) - 1; i >= 0; i-- {
		if words[i] == target {
			return i
		}
	}
	return -1
}

// firstIndexFrom returns the index of the first word in words[start:end] that
// is in the given set, or -1 if none is.
func firstIndexFrom(words []string, start, end int, set map[string]bool) int {
	if start < 0 {
		start = 0
	}
	if start > len(words) {
		start = len(words)
	}
	if end > len(words) {
		end = len(words)
	}
	for i := start; i < end; i++ {
		if set[words[i]] {
			return i
		}
	}
	return -1
}

// keywords are the SQL keywords offered for completion.
var keywords = []string{
	"SELECT", "FROM", "WHERE", "JOIN", "INNER JOIN", "LEFT JOIN", "RIGHT JOIN",
	"GROUP BY", "ORDER BY", "LIMIT", "HAVING", "AS", "AND", "OR", "NOT", "IN",
	"IS", "NULL", "BETWEEN", "DISTINCT", "ON", "USING", "BY", "DESC", "ASC",
	"OFFSET", "UNION", "WITH", "COUNT", "SUM", "AVG", "MIN", "MAX", "EXISTS",
	"DELETE", "UPDATE", "INSERT", "CREATE", "DROP",
}

// clauseKeywords are keywords that terminate the FROM/JOIN table list and are
// themselves completable once typing starts.
var clauseKeywords = map[string]bool{
	"WHERE": true, "GROUP": true, "ORDER": true, "LIMIT": true, "HAVING": true,
	"UNION": true, "ON": true, "AND": true, "OR": true, "BY": true, "USING": true,
	"AS": true, "DESC": true, "ASC": true, "OFFSET": true,
}
