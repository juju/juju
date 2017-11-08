// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

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

	// Sort by resource name
	names, resourcesByName := groupCharmResourcesByName(resources)

	// To format things into columns.
	tw := output.TabWriter(writer)

	// Write the header.
	// We do not print a section label.
	fmt.Fprintln(tw, "Resource\tRevision")

	// Print each info to its own row.
	for _, name := range names {
		for _, res := range resourcesByName[name] {
			// the column headers must be kept in sync with these.
			fmt.Fprintf(tw, "%s\t%d\n",
				name,
				res.Revision,
			)
		}
	}
	tw.Flush()

	return nil
}

func groupCharmResourcesByName(resources []FormattedCharmResource) ([]string, map[string][]FormattedCharmResource) {
	// Sort by resource name
	names := make([]string, len(resources))
	resourcesByName := map[string][]FormattedCharmResource{}
	for i, r := range resources {
		names[i] = r.Name
		allNamedResources, ok := resourcesByName[r.Name]
		if !ok {
			allNamedResources = []FormattedCharmResource{}
		}
		resourcesByName[r.Name] = append(allNamedResources, r)
	}
	sort.Strings(names)
	return names, resourcesByName
}

// FormatAppTabular returns a tabular summary of resources.
func FormatAppTabular(writer io.Writer, value interface{}) error {
	switch resources := value.(type) {
	case FormattedApplicationInfo:
		formatApplicationTabular(writer, resources)
		return nil
	case []FormattedAppResource:
		formatUnitTabular(writer, resources)
		return nil
	case FormattedApplicationDetails:
		formatApplicationDetailTabular(writer, resources)
		return nil
	case FormattedUnitDetails:
		formatUnitDetailTabular(writer, resources)
		return nil
	default:
		return errors.Errorf("unexpected type for data: %T", resources)
	}
}

func formatApplicationTabular(writer io.Writer, info FormattedApplicationInfo) {
	// Sort by resource name
	names, resourcesByName := groupApplicationResourcesByName(info.Resources)

	tw := output.TabWriter(writer)
	fmt.Fprintln(tw, "Resource\tSupplied by\tRevision")

	// Print each info to its own row.
	for _, name := range names {
		for _, r := range resourcesByName[name] {
			// the column headers must be kept in sync with these.
			fmt.Fprintf(tw, "%v\t%v\t%v\n",
				r.Name,
				r.CombinedOrigin,
				r.CombinedRevision,
			)
		}
	}

	// Don't forget to flush!  The Tab writer won't actually write to the output
	// until you flush, which would then have its output incorrectly ordered
	// with the below fmt.Fprintlns.
	tw.Flush()

	writeUpdates(info.Updates, writer, tw)
}

func groupApplicationResourcesByName(resources []FormattedAppResource) ([]string, map[string][]FormattedAppResource) {
	// Sort by resource name
	names := make([]string, len(resources))
	resourcesByName := map[string][]FormattedAppResource{}
	for i, r := range resources {
		names[i] = r.Name
		allNamedResources, ok := resourcesByName[r.Name]
		if !ok {
			allNamedResources = []FormattedAppResource{}
		}
		resourcesByName[r.Name] = append(allNamedResources, r)
	}
	sort.Strings(names)
	return names, resourcesByName
}

func writeUpdates(updates []FormattedCharmResource, out io.Writer, tw *ansiterm.TabWriter) {
	names, resourcesByName := groupCharmResourcesByName(updates)

	if len(updates) > 0 {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "[Updates Available]")
		fmt.Fprintln(tw, "Resource\tRevision")
		for _, name := range names {
			for _, r := range resourcesByName[name] {
				fmt.Fprintf(tw, "%v\t%v\n",
					name,
					r.Revision,
				)
			}
		}
	}

	tw.Flush()
}

func formatUnitTabular(writer io.Writer, resources []FormattedAppResource) {
	names, resourcesByName := groupApplicationResourcesByName(resources)

	// To format things into columns.
	tw := output.TabWriter(writer)

	// Write the header.
	// We do not print a section label.
	fmt.Fprintln(tw, "Resource\tRevision")

	// Print each info to its own row.
	for _, name := range names {
		for _, r := range resourcesByName[name] {
			// the column headers must be kept in sync with these.
			fmt.Fprintf(tw, "%v\t%v\n",
				r.Name,
				r.CombinedRevision,
			)
		}
	}
	tw.Flush()
}

func formatApplicationDetailTabular(writer io.Writer, resources FormattedApplicationDetails) {
	// note that the unit resource can be a zero value here, to indicate that
	// the unit has not downloaded that resource yet.
	sort.Sort(byUnitID(resources.Resources))
	// To format things into columns.
	tw := output.TabWriter(writer)

	// Write the header.
	fmt.Fprintln(tw, "Unit\tResource\tRevision\tExpected")

	for _, r := range resources.Resources {
		fmt.Fprintf(tw, "%v\t%v\t%v\t%v\n",
			r.UnitID,
			r.Expected.Name,
			r.Unit.CombinedRevision,
			r.RevProgress,
		)
	}
	tw.Flush()

	writeUpdates(resources.Updates, writer, tw)
}

func formatUnitDetailTabular(writer io.Writer, resources FormattedUnitDetails) {
	// note that the unit resource can be a zero value here, to indicate that
	// the unit has not downloaded that resource yet.
	sort.Sort(byUnitID(resources))
	// To format things into columns.
	tw := output.TabWriter(writer)

	// Write the header.
	fmt.Fprintln(tw, "Resource\tRevision\tExpected")

	for _, r := range resources {
		fmt.Fprintf(tw, "%v\t%v\t%v\n",
			r.Expected.Name,
			r.Unit.CombinedRevision,
			r.RevProgress,
		)
	}
	tw.Flush()
}

type byUnitID []FormattedDetailResource

func (b byUnitID) Len() int      { return len(b) }
func (b byUnitID) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

func (b byUnitID) Less(i, j int) bool {
	if b[i].UnitNumber != b[j].UnitNumber {
		return b[i].UnitNumber < b[j].UnitNumber
	}
	return b[i].Expected.Name < b[j].Expected.Name
}
