// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
)

// DumpTable dumps the contents of the given table to stdout.
// This is useful for debugging tests. It is not intended for use
// in production code.
func DumpTable(c *tc.C, db *sql.DB, table string, extraTables ...string) {
	for _, t := range append([]string{table}, extraTables...) {
		rows, err := db.Query(fmt.Sprintf("SELECT * FROM %q", t))
		c.Assert(err, jc.ErrorIsNil)
		defer rows.Close()

		cols, err := rows.Columns()
		c.Assert(err, jc.ErrorIsNil)

		buffer := new(bytes.Buffer)
		writer := tabwriter.NewWriter(buffer, 0, 8, 4, ' ', 0)
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
			fmt.Fprintln(writer)
		}
		err = rows.Err()
		c.Assert(err, jc.ErrorIsNil)
		writer.Flush()

		fmt.Fprintf(os.Stdout, "Table - %s:\n", t)

		var width int
		scanner := bufio.NewScanner(bytes.NewBuffer(buffer.Bytes()))
		for scanner.Scan() {
			if num := len(scanner.Text()); num > width {
				width = num
			}
		}

		fmt.Fprintln(os.Stdout, strings.Repeat("-", width-4))
		fmt.Fprintln(os.Stdout, buffer.String())
		fmt.Fprintln(os.Stdout, strings.Repeat("-", width-4))
		fmt.Fprintln(os.Stdout)
	}
}
