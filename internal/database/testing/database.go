// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"database/sql"
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
