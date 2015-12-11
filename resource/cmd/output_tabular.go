// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"

	"github.com/juju/juju/resource"
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
	infos, valueConverted := value.([]FormattedInfo)
	if !valueConverted {
		return nil, errors.Errorf("expected value of type %T, got %T", infos, value)
	}

	// TODO(ericsnow) sort the rows first?

	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)

	// Write the header.
	// We do not print a section label.
	fmt.Fprintln(tw, tabularHeader)

	// Print each info to its own row.
	for _, info := range infos {
		rev := "-"
		if info.Origin == resource.OriginKindStore.String() {
			rev = fmt.Sprintf("%d", info.Revision)
		}
		// tabularColumns must be kept in sync with these.
		fmt.Fprintf(tw, tabularRow+"\n",
			info.Name,
			info.Origin,
			rev,
			info.Comment,
		)
	}
	tw.Flush()

	return out.Bytes(), nil
}
