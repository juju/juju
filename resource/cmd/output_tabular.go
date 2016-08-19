// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"fmt"
	"io"
	"sort"

	"github.com/juju/ansiterm"
	"github.com/juju/errors"
	"github.com/juju/juju/cmd/output"
)

// FormatCharmTabular returns a tabular summary of charm resources.
func FormatCharmTabular(writer io.Writer, value interface{}) error {
	resources, valueConverted := value.([]FormattedCharmResource)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", resources, value)
	}

	// TODO(ericsnow) sort the rows first?

	// To format things into columns.
	tw := output.TabWriter(writer)

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

	return nil
}

// FormatSvcTabular returns a tabular summary of resources.
func FormatSvcTabular(writer io.Writer, value interface{}) error {
	switch resources := value.(type) {
	case FormattedServiceInfo:
		formatServiceTabular(writer, resources)
		return nil
	case []FormattedUnitResource:
		formatUnitTabular(writer, resources)
		return nil
	case FormattedServiceDetails:
		formatServiceDetailTabular(writer, resources)
		return nil
	case FormattedUnitDetails:
		formatUnitDetailTabular(writer, resources)
		return nil
	default:
		return errors.Errorf("unexpected type for data: %T", resources)
	}
}

func formatServiceTabular(writer io.Writer, info FormattedServiceInfo) {
	// TODO(ericsnow) sort the rows first?

	fmt.Fprintln(writer, "[Service]")
	tw := output.TabWriter(writer)
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

	writeUpdates(info.Updates, writer, tw)
}

func writeUpdates(updates []FormattedCharmResource, out io.Writer, tw *ansiterm.TabWriter) {
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

func formatUnitTabular(writer io.Writer, resources []FormattedUnitResource) {
	// TODO(ericsnow) sort the rows first?

	fmt.Fprintln(writer, "[Unit]")

	// To format things into columns.
	tw := output.TabWriter(writer)

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
}

func formatServiceDetailTabular(writer io.Writer, resources FormattedServiceDetails) {
	// note that the unit resource can be a zero value here, to indicate that
	// the unit has not downloaded that resource yet.

	fmt.Fprintln(writer, "[Units]")

	sort.Sort(byUnitID(resources.Resources))
	// To format things into columns.
	tw := output.TabWriter(writer)

	// Write the header.
	fmt.Fprintln(tw, "UNIT\tRESOURCE\tREVISION\tEXPECTED")

	for _, r := range resources.Resources {
		fmt.Fprintf(tw, "%v\t%v\t%v\t%v\n",
			r.unitNumber,
			r.Expected.Name,
			r.Unit.combinedRevision,
			r.revProgress,
		)
	}
	tw.Flush()

	writeUpdates(resources.Updates, writer, tw)
}

func formatUnitDetailTabular(writer io.Writer, resources FormattedUnitDetails) {
	// note that the unit resource can be a zero value here, to indicate that
	// the unit has not downloaded that resource yet.

	fmt.Fprintln(writer, "[Unit]")

	sort.Sort(byUnitID(resources))
	// To format things into columns.
	tw := output.TabWriter(writer)

	// Write the header.
	fmt.Fprintln(tw, "RESOURCE\tREVISION\tEXPECTED")

	for _, r := range resources {
		fmt.Fprintf(tw, "%v\t%v\t%v\n",
			r.Expected.Name,
			r.Unit.combinedRevision,
			r.revProgress,
		)
	}
	tw.Flush()
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
