// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"fmt"
	"text/tabwriter"

	"github.com/juju/errors"
)

// FormatCharmTabular returns a tabular summary of charm resources.
func FormatCharmTabular(value interface{}) ([]byte, error) {
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
	fmt.Fprintln(tw, "RESOURCE\tFROM\tREV\tCOMMENT")

	// Print each info to its own row.
	for _, res := range resources {
		if res.Origin == "store" {
			res.Origin = "charmstore"
		}

		// the column headers must be kept in sync with these.
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			res.Name,
			res.Origin,
			res.charmRevision,
			res.Comment,
		)
	}
	tw.Flush()

	return out.Bytes(), nil
}

// FormatSvcTabular returns a tabular summary of resources.
func FormatSvcTabular(value interface{}) ([]byte, error) {
	resources, valueConverted := value.([]FormattedSvcResource)
	if !valueConverted {
		return nil, errors.Errorf("expected value of type %T, got %T", resources, value)
	}

	// TODO(ericsnow) sort the rows first?

	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)

	// Write the header.
	// We do not print a section label.
	fmt.Fprintln(tw, "RESOURCE\tORIGIN\tREV\tUSED\tCOMMENT")

	// Print each info to its own row.
	for _, r := range resources {
		// the column headers must be kept in sync with these.
		fmt.Fprintf(tw, "%v\t%v\t%v\t%v\t%v\n",
			r.Name,
			r.combinedOrigin,
			r.combinedRevision,
			r.usedYesNo,
			r.Comment,
		)
	}
	tw.Flush()

	return out.Bytes(), nil
}
