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

const section = "[Unit Payloads]"

var columns = []string{
	"unit",
	"machine",
	"payload-class",
	"status",
	"type",
	"id",
	"tags",
}

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
	row := func(values ...string) {
		for _, v := range values {
			fmt.Fprintf(tw, "%s\t", v)
		}
		fmt.Fprintln(tw)
	}

	// Write the header.
	fmt.Fprintln(tw, section)
	var labels []string
	for _, name := range columns {
		labels = append(labels, strings.ToUpper(name))
	}
	row(labels...)

	// Print each payload to its own row.
	for _, payload := range payloads {
		values := payload.strings(columns...)
		row(values...)
	}
	tw.Flush()

	return out.Bytes(), nil
}
