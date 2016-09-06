// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"io"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/cmd/output"
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

// FormatTabular writes a tabular summary of payloads.
func FormatTabular(writer io.Writer, value interface{}) error {
	payloads, valueConverted := value.([]FormattedPayload)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", payloads, value)
	}

	// TODO(ericsnow) sort the rows first?

	tw := output.TabWriter(writer)

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

	return nil
}
