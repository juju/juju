// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

// DumpTables dumps the contents of the given tables to stdout.
// This is useful for debugging tests. It is not intended for use
// in production code.
func DumpTables(c *gc.C, db *sql.DB, tables ...string) {
	for _, table := range tables {
		dumpTable(c, db, table, newTabularFormatter())
	}
}

// DumpTablesJSON prints the named tables to stdout in a JSON-like format. The
// output of this function is not, and is not intended to be, valid JSON.
// This is intended as a debugging tool for state tests. It is not intended for
// use in production code.
func DumpTablesJSON(c *gc.C, db *sql.DB, tables ...string) {
	for _, table := range tables {
		dumpTable(c, db, table, newJSONFormatter(c))
	}
}

func dumpTable(c *gc.C, db *sql.DB, tableName string, f formatter) {
	f.SetTableName(tableName)
	rows, err := db.Query("SELECT * FROM " + tableName)
	if err != nil {
		// Soft fail, most likely the table doesn't exist
		fmt.Println(err)
		return
	}

	columns, err := rows.Columns()
	c.Assert(err, jc.ErrorIsNil)
	f.SetColumns(columns)

	for rows.Next() {
		row := make([]any, len(columns))
		for i := range row {
			row[i] = new(any)
		}
		err = rows.Scan(row...)
		c.Assert(err, jc.ErrorIsNil)
		f.AddRow(row)
	}
	f.Flush()
}

// formatter defines a way of printing a table.
type formatter interface {
	// SetTableName defines the name of the table
	SetTableName(string)
	// SetColumns defines the columns of the table.
	SetColumns([]string)
	// AddRow adds a row to the table
	AddRow([]any)
	// Flush prints the table.
	Flush()
}

// tabularFormatter formats a table in a tabular format.
type tabularFormatter struct {
	tableName string
	tw        *tabwriter.Writer
}

func newTabularFormatter() *tabularFormatter {
	return &tabularFormatter{
		tw: tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', 0),
	}
}

func (t *tabularFormatter) SetTableName(name string) {
	t.tableName = name
}

func (t *tabularFormatter) SetColumns(cols []string) {
	for _, col := range cols {
		fmt.Fprintf(t.tw, "%s\t", col)
	}
	fmt.Fprintln(t.tw)
}

func (t *tabularFormatter) AddRow(vals []any) {
	for _, val := range vals {
		fmt.Fprintf(t.tw, "%v\t", *val.(*any))
	}
}

func (t *tabularFormatter) Flush() {
	fmt.Fprintln(os.Stdout)
	fmt.Printf("***** TABLE %q *****\n", t.tableName)
	t.tw.Flush()
	fmt.Fprintln(os.Stdout)
}

// jsonFormatter formats a table in a JSON-like format. The output of this
// formatter is not, and is not intended to be, valid JSON.
type jsonFormatter struct {
	c           *gc.C
	tableName   string
	columnNames []string
	rows        [][]any
}

func newJSONFormatter(c *gc.C) *jsonFormatter {
	return &jsonFormatter{
		c: c,
	}
}

func (j *jsonFormatter) SetTableName(name string) {
	j.tableName = name
}

func (j *jsonFormatter) SetColumns(cols []string) {
	j.columnNames = cols
}

func (j *jsonFormatter) AddRow(row []any) {
	j.rows = append(j.rows, row)
}

func (j *jsonFormatter) Flush() {
	fmt.Printf("***** TABLE %q *****\n", j.tableName)
	stdout := json.NewEncoder(os.Stdout)

	for _, row := range j.rows {
		jsonRow := map[string]any{}
		for i, columnName := range j.columnNames {
			jsonRow[columnName] = row[i]
		}

		stdout.SetIndent("", "  ")
		err := stdout.Encode(jsonRow)
		j.c.Assert(err, jc.ErrorIsNil)
	}
	fmt.Println()
}
