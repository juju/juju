// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
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
	resources, valueConverted := value.([]FormattedCharmResource)
	if !valueConverted {
		return nil, errors.Errorf("expected value of type %T, got %T", resources, value)
	}

	// TODO(ericsnow) sort the rows first?

	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)

	// Write the header.
	// We do not print a section label.
	fmt.Fprintln(tw, tabularHeader)

	// Print each info to its own row.
	for _, res := range resources {
		rev := "-"
		if res.Origin == charmresource.OriginStore.String() {
			rev = fmt.Sprintf("%d", res.Revision)
		}
		// tabularColumns must be kept in sync with these.
		fmt.Fprintf(tw, tabularRow+"\n",
			res.Name,
			res.Origin,
			rev,
			res.Comment,
		)
	}
	tw.Flush()

	return out.Bytes(), nil
}
