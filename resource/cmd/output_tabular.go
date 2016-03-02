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
	case FormattedServiceInfo:
		return formatServiceTabular(resources), nil
	case []FormattedUnitResource:
		return formatUnitTabular(resources), nil
	case FormattedServiceDetails:
		return formatServiceDetailTabular(resources), nil
	case FormattedUnitDetails:
		return formatUnitDetailTabular(resources), nil
	default:
		return nil, errors.Errorf("unexpected type for data: %T", resources)
	}
}

func formatServiceTabular(info FormattedServiceInfo) []byte {
	// TODO(ericsnow) sort the rows first?

	var out bytes.Buffer

	fmt.Fprintln(&out, "[Service]")
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)
	fmt.Fprintln(tw, "RESOURCE\tSUPPLIED BY\tREVISION")

	// Print each info to its own row.
	for _, r := range info.Resources {
		// the column headers must be kept in sync with these.
		fmt.Fprintf(tw, "%v\t%v\t%v\n",
			r.Name,
			r.combinedOrigin,
			r.combinedRevision,
		)
	}

	// Don't forget to flush!  The Tab writer won't actually write to the output
	// until you flush, which would then have its output incorrectly ordered
	// with the below fmt.Fprintlns.
	tw.Flush()

	writeUpdates(info.Updates, &out, tw)

	return out.Bytes()
}

func writeUpdates(updates []FormattedCharmResource, out *bytes.Buffer, tw *tabwriter.Writer) {
	if len(updates) > 0 {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "[Updates Available]")
		fmt.Fprintln(tw, "RESOURCE\tREVISION")
		for _, r := range updates {
			fmt.Fprintf(tw, "%v\t%v\n",
				r.Name,
				r.Revision,
			)
		}
	}

	tw.Flush()
}

func formatUnitTabular(resources []FormattedUnitResource) []byte {
	// TODO(ericsnow) sort the rows first?

	var out bytes.Buffer

	fmt.Fprintln(&out, "[Unit]")

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

func formatServiceDetailTabular(resources FormattedServiceDetails) []byte {
	// note that the unit resource can be a zero value here, to indicate that
	// the unit has not downloaded that resource yet.

	var out bytes.Buffer
	fmt.Fprintln(&out, "[Units]")

	sort.Sort(byUnitID(resources.Resources))
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)

	// Write the header.
	fmt.Fprintln(tw, "UNIT\tRESOURCE\tREVISION\tEXPECTED")

	for _, r := range resources.Resources {
		fmt.Fprintf(tw, "%v\t%v\t%v\t%v\n",
			r.unitNumber,
			r.Expected.Name,
			r.Unit.combinedRevision,
			r.Expected.combinedRevision,
		)
	}
	tw.Flush()

	writeUpdates(resources.Updates, &out, tw)

	return out.Bytes()
}

func formatUnitDetailTabular(resources FormattedUnitDetails) []byte {
	// note that the unit resource can be a zero value here, to indicate that
	// the unit has not downloaded that resource yet.

	var out bytes.Buffer
	fmt.Fprintln(&out, "[Unit]")

	sort.Sort(byUnitID(resources))
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)

	// Write the header.
	fmt.Fprintln(tw, "RESOURCE\tREVISION\tEXPECTED")

	for _, r := range resources {
		fmt.Fprintf(tw, "%v\t%v\t%v\n",
			r.Expected.Name,
			r.Unit.combinedRevision,
			r.Expected.combinedRevision,
		)
	}
	tw.Flush()
	return out.Bytes()
}

type byUnitID []FormattedDetailResource

func (b byUnitID) Len() int      { return len(b) }
func (b byUnitID) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

func (b byUnitID) Less(i, j int) bool {
	if b[i].unitNumber != b[j].unitNumber {
		return b[i].unitNumber < b[j].unitNumber
	}
	return b[i].Expected.Name < b[j].Expected.Name
}
