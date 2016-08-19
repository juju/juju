// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package output

import (
	"io"
	"text/tabwriter"

	"github.com/juju/cmd"
)

// DefaultFormatters holds the formatters that can be
// specified with the --format flag.
var DefaultFormatters = map[string]cmd.Formatter{
	"yaml": cmd.FormatYaml,
	"json": cmd.FormatJson,
}

// TabWriter returns a new tab writer with common layout definition.
func TabWriter(writer io.Writer) *tabwriter.Writer {
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	return tabwriter.NewWriter(writer, minwidth, tabwidth, padding, padchar, flags)
}
