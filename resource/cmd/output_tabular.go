// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"fmt"
	"sort"
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
	fmt.Fprintln(tw, "RESOURCE\tREVISION")

	// Print each info to its own row.
	for _, res := range resources {
		// the column headers must be kept in sync with these.
		fmt.Fprintf(tw, "%s\t%d\n",
			res.Name,
			res.Revision,
		)
	}
	tw.Flush()

	return out.Bytes(), nil
}

// FormatSvcTabular returns a tabular summary of resources.
func FormatSvcTabular(value interface{}) ([]byte, error) {
	switch resources := value.(type) {
	case []FormattedSvcResource:
		return formatServiceTabular(resources), nil
	case []FormattedUnitResource:
		return formatUnitTabular(resources), nil
	case []FormattedDebugUnitResource:
		return formatDebugUnitTabular(resources), nil
	default:
		return nil, errors.Errorf("unexpected type for data: %T", resources)
	}
}

func formatServiceTabular(resources []FormattedSvcResource) []byte {
	// TODO(ericsnow) sort the rows first?

	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)

	// Write the header.
	// We do not print a section label.
	fmt.Fprintln(tw, "RESOURCE\tSUPPLIED BY\tREVISION")

	// Print each info to its own row.
	for _, r := range resources {
		// the column headers must be kept in sync with these.
		fmt.Fprintf(tw, "%v\t%v\t%v\n",
			r.Name,
			r.combinedOrigin,
			r.combinedRevision,
		)
	}
	tw.Flush()

	return out.Bytes()
}

func formatUnitTabular(resources []FormattedUnitResource) []byte {
	// TODO(ericsnow) sort the rows first?

	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)

	// Write the header.
	// We do not print a section label.
	fmt.Fprintln(tw, "RESOURCE\tREVISION")

	// Print each info to its own row.
	for _, r := range resources {
		// the column headers must be kept in sync with these.
		fmt.Fprintf(tw, "%v\t%v\n",
			r.Name,
			r.combinedRevision,
		)
	}
	tw.Flush()

	return out.Bytes()
}

func formatDebugUnitTabular(resources []FormattedDebugUnitResource) []byte {
	// TODO(ericsnow) sort the rows first?
	sort.Sort(byUnitID(resources))

	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)

	// Write the header.
	fmt.Fprintln(tw, "UNIT\tRESOURCE\tREVISION\tEXPECTED")

	for _, r := range resources {
		fmt.Fprintf(tw, "%v\t%v\t%v\t%v\n",
			r.unitNumber,
			r.Expected.Name,
			r.Unit.combinedRevision,
			r.Expected.combinedRevision,
		)
	}
	tw.Flush()
	return out.Bytes()
}

type byUnitID []FormattedDebugUnitResource

func (b byUnitID) Len() int      { return len(b) }
func (b byUnitID) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

func (b byUnitID) Less(i, j int) bool {
	if b[i].unitNumber != b[j].unitNumber {
		return b[i].unitNumber < b[j].unitNumber
	}
	return b[i].Expected.Name < b[j].Expected.Name
}
