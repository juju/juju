// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

var (
	tabularColumns = []string{
		"RESOURCE",
		"FROM",
		"REV",
		"COMMENT",
	}

	tabularHeader = strings.Join(tabularColumns, "\t") + "\t"
	tabularRow    = strings.Repeat("%s\t", len(tabularColumns))
)

// FormatTabular returns a tabular summary of payloads.
func FormatTabular(value interface{}) ([]byte, error) {
	specs, valueConverted := value.([]FormattedSpec)
	if !valueConverted {
		return nil, errors.Errorf("expected value of type %T, got %T", specs, value)
	}

	// TODO(ericsnow) sort the rows first?

	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)

	// Write the header.
	// We do not print a section label.
	fmt.Fprintln(tw, tabularHeader)

	// Print each spec to its own row.
	for _, spec := range specs {
		rev := spec.Revision
		if rev == "" {
			rev = "-"
		}
		// tabularColumns must be kept in sync with these.
		fmt.Fprintf(tw, tabularRow+"\n",
			spec.Name,
			spec.Origin,
			rev,
			spec.Comment,
		)
	}
	tw.Flush()

	return out.Bytes(), nil
}
