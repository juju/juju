// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
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

	tabularColumns := []string{
		"RESOURCE",
		"FROM",
		"REV",
		"COMMENT",
	}

	// Write the header.
	// We do not print a section label.
	fmt.Fprintln(tw, strings.Join(tabularColumns, "\t"))

	// Print each info to its own row.
	for _, res := range resources {
		rev := "-"
		if res.Origin == OriginStore {
			rev = fmt.Sprintf("%d", res.Revision)
		}
		// tabularColumns must be kept in sync with these.
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			res.Name,
			res.Origin.lower(),
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

	sort.Sort(byNameRev(resources))

	// TODO(ericsnow) sort the rows first?

	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)

	tabularColumns := []string{
		"RESOURCE",
		"ORIGIN",
		"REV",
		"USED",
		"COMMENT",
	}

	// Write the header.
	// We do not print a section label.
	fmt.Fprintln(tw, strings.Join(tabularColumns, "\t"))

	// Print each info to its own row.
	for _, r := range resources {
		// tabularColumns must be kept in sync with these.
		fmt.Fprintf(tw, "%v\t%v\t%v\t%v\t%v\n",
			r.Name,
			tabularOrigin(r),
			tabularRev(r),
			tabularUsed(r.Used),
			r.Comment,
		)
	}
	tw.Flush()

	return out.Bytes(), nil
}

func tabularRev(r FormattedSvcResource) interface{} {
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
		return r.Origin.lower()
	}
}

func tabularUsed(used bool) string {
	if used {
		return "yes"
	}
	return "no"
}

// byNameRev sorts the resources by name and then by revision/timestamp.
type byNameRev []FormattedSvcResource

func (b byNameRev) Len() int {
	return len(b)
}

func (b byNameRev) Less(i, j int) bool {
	if b[i].Name < b[j].Name {
		return true
	}
	if b[i].Name > b[j].Name {
		return false
	}

	// Sort revisions and timestamps descending, so most recent ones are at the
	// top.

	if b[i].Revision > b[j].Revision {
		return true
	}
	if b[i].Revision < b[j].Revision {
		return false
	}
	return b[i].Timestamp.After(b[j].Timestamp)
}

func (b byNameRev) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
