// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

const tabularSection = "[Unit Payloads]"

var (
	tabularColumns = []string{
		"UNIT",
		"MACHINE",
		"PAYLOAD-CLASS",
		"STATUS",
		"TYPE",
		"ID",
		"TAGS", // TODO(ericsnow) Chane this to "LABELS"?
	}

	tabularHeader = strings.Join(tabularColumns, "\t") + "\t"
	tabularRow    = strings.Repeat("%s\t", len(tabularColumns))
)

// FormatTabular returns a tabular summary of payloads.
func FormatTabular(value interface{}) ([]byte, error) {
	payloads, valueConverted := value.([]FormattedPayload)
	if !valueConverted {
		return nil, errors.Errorf("expected value of type %T, got %T", payloads, value)
	}

	// TODO(ericsnow) sort the rows first?

	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)

	// Write the header.
	fmt.Fprintln(tw, tabularSection)
	fmt.Fprintln(tw, tabularHeader)

	// Print each payload to its own row.
	for _, payload := range payloads {
		// tabularColumns must be kept in sync with these.
		fmt.Fprintf(tw, tabularRow+"\n",
			payload.Unit,
			payload.Machine,
			payload.Class,
			payload.Status,
			payload.Type,
			payload.ID,
			strings.Join(payload.Labels, " "),
		)
	}
	tw.Flush()

	return out.Bytes(), nil
}
