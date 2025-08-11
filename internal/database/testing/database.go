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
)

// DumpTable dumps the contents of the given table to stdout.
// This is useful for debugging tests. It is not intended for use
// in production code.
func DumpTable(c *tc.C, db *sql.DB, table string, extraTables ...string) {
	for _, t := range append([]string{table}, extraTables...) {
		rows, err := db.Query(fmt.Sprintf("SELECT * FROM %q", t))
		c.Assert(err, tc.ErrorIsNil)
		defer func() { _ = rows.Close() }()

		cols, err := rows.Columns()
		c.Assert(err, tc.ErrorIsNil)

		buffer := new(bytes.Buffer)
		writer := tabwriter.NewWriter(buffer, 0, 8, 4, ' ', 0)
		for _, col := range cols {
			_, _ = fmt.Fprintf(writer, "%s\t", col)
		}

		_, _ = fmt.Fprintln(writer)

		vals := make([]any, len(cols))
		for i := range vals {
			vals[i] = new(any)
		}

		for rows.Next() {
			err = rows.Scan(vals...)
			c.Assert(err, tc.ErrorIsNil)

			for _, val := range vals {
				_, _ = fmt.Fprintf(writer, "%v\t", *val.(*any))
			}
			_, _ = fmt.Fprintln(writer)
		}
		err = rows.Err()
		c.Assert(err, tc.ErrorIsNil)
		_ = writer.Flush()

		_, _ = fmt.Fprintf(os.Stdout, "Table - %s:\n", t)

		var width int
		scanner := bufio.NewScanner(bytes.NewBuffer(buffer.Bytes()))
		for scanner.Scan() {
			if num := len(scanner.Text()); num > width {
				width = num
			}
		}

		_, _ = fmt.Fprintln(os.Stdout, strings.Repeat("-", width-4))
		_, _ = fmt.Fprintln(os.Stdout, buffer.String())
		_, _ = fmt.Fprintln(os.Stdout, strings.Repeat("-", width-4))
		_, _ = fmt.Fprintln(os.Stdout)
	}
}
