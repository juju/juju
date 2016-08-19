// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package output

import (
	"io"

	"github.com/juju/ansiterm"
	"github.com/juju/cmd"
)

// DefaultFormatters holds the formatters that can be
// specified with the --format flag.
var DefaultFormatters = map[string]cmd.Formatter{
	"yaml": cmd.FormatYaml,
	"json": cmd.FormatJson,
}

// TabWriter returns a new tab writer with common layout definition.
func TabWriter(writer io.Writer) *ansiterm.TabWriter {
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	return ansiterm.NewTabWriter(writer, minwidth, tabwidth, padding, padchar, flags)
}

// CurrentHighlight is the color used to show the current
// controller, user or model in tabular output.
var CurrentHighlight = ansiterm.Foreground(ansiterm.Green)
