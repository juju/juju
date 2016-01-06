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
		rev := "-"
		if res.Origin == OriginStore {
			rev = fmt.Sprintf("%d", res.Revision)
		}
		// the column headers must be kept in sync with these.
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			res.Name,
			res.Origin,
			rev,
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
			tabularOrigin(r),
			tabularRevision(r),
			tabularUsed(r.Used),
			r.Comment,
		)
	}
	tw.Flush()

	return out.Bytes(), nil
}

func tabularRevision(r FormattedSvcResource) interface{} {
	switch r.Origin {
	case OriginStore:
		return r.Revision
	case OriginUpload:
		if !r.Timestamp.IsZero() {
			return r.Timestamp.Format("2006-02-01")
		}
	}
	return "-"
}

func tabularOrigin(r FormattedSvcResource) string {
	switch r.Origin {
	case OriginUpload:
		return r.Username
	default:
		return string(r.Origin)
	}
}

func tabularUsed(used bool) string {
	if used {
		return "yes"
	}
	return "no"
}
