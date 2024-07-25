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

// DumpTable dumps the contents of the given table to stdout.
// This is useful for debugging tests. It is not intended for use
// in production code.
func DumpTable(c *gc.C, db *sql.DB, table string) {
	rows, err := db.Query("SELECT * FROM " + table)
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	cols, err := rows.Columns()
	c.Assert(err, jc.ErrorIsNil)

	fmt.Fprintln(os.Stdout)

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', 0)
	for _, col := range cols {
		fmt.Fprintf(writer, "%s\t", col)
	}
	fmt.Fprintln(writer)

	vals := make([]any, len(cols))
	for i := range vals {
		vals[i] = new(any)
	}

	for rows.Next() {
		err = rows.Scan(vals...)
		c.Assert(err, jc.ErrorIsNil)

		for _, val := range vals {
			fmt.Fprintf(writer, "%v\t", *val.(*any))
		}
	}
	err = rows.Err()
	c.Assert(err, jc.ErrorIsNil)

	writer.Flush()
	fmt.Fprintln(os.Stdout)
}

// DumpTablesJSON prints the named tables to stdout in a JSON format. This is
// intended as a debugging tool for state tests. It is not intended for use in
// production code.
func DumpTablesJSON(c *gc.C, db *sql.DB, tableNames ...string) {
	for _, tableName := range tableNames {
		query := fmt.Sprintf("SELECT * FROM %s", tableName)
		table := &tableData{
			tableName: tableName,
		}

		rows, err := db.Query(query)
		if err != nil {
			// Soft fail, most likely the table doesn't exist
			fmt.Println(err)
			continue
		}

		columns, err := rows.Columns()
		c.Assert(err, jc.ErrorIsNil)
		table.columnNames = columns

		for rows.Next() {
			row := make([]any, len(columns))
			for i := range row {
				row[i] = new(any)
			}
			err = rows.Scan(row...)
			c.Assert(err, jc.ErrorIsNil)
			table.rows = append(table.rows, row)
		}

		printJSON(c, table)
	}

}

// tableData is an intermediate representation of an arbitrary table, ready to
// be processed for printing.
type tableData struct {
	tableName   string
	columnNames []string
	rows        [][]any
}

func printJSON(c *gc.C, table *tableData) {
	fmt.Printf("***** TABLE %q *****\n", table.tableName)
	stdout := json.NewEncoder(os.Stdout)

	for _, row := range table.rows {
		jsonRow := map[string]any{}
		for i, columnName := range table.columnNames {
			jsonRow[columnName] = row[i]
		}

		stdout.SetIndent("", "  ")
		err := stdout.Encode(jsonRow)
		c.Assert(err, jc.ErrorIsNil)
	}
	fmt.Println()
}
