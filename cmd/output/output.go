// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package output

import (
	"fmt"
	"io"

	"github.com/juju/ansiterm"
	"github.com/juju/cmd"
	"github.com/juju/juju/status"
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

// TabWriterPrint returns a function that is used to help print many
// elements to a tab writer.
func TabWriterPrint(tw *ansiterm.TabWriter) func(...interface{}) {
	w := Wrapper{tw}
	return w.Print
}

// TabWriterPrintln returns a function that is used to help print many
// elements to a tab writer, and finishes with a new line.
func TabWriterPrintln(tw *ansiterm.TabWriter) func(...interface{}) {
	w := Wrapper{tw}
	return w.Println
}

// TabWriterPrintColor returns a function that will print out a single value
// followed by a tab in the color context specified.
func TabWriterPrintColor(tw *ansiterm.TabWriter) func(*ansiterm.Context, interface{}) {
	w := Wrapper{tw}
	return w.PrintColor
}

// Wrapper provides some helper functions for writing values out tab separated.
type Wrapper struct {
	*ansiterm.TabWriter
}

// Print writes each value followed by a tab.
func (w *Wrapper) Print(values ...interface{}) {
	for _, v := range values {
		fmt.Fprintf(w, "%v\t", v)
	}
}

// Println writes many tab separated values finished with a new line.
func (w *Wrapper) Println(values ...interface{}) {
	for i, v := range values {
		if i != len(values)-1 {
			fmt.Fprintf(w, "%v\t", v)
		} else {
			fmt.Fprintf(w, "%v", v)
		}
	}
	fmt.Fprintln(w)
}

// PrintColor writes the value out in the color context specified.
func (w *Wrapper) PrintColor(ctx *ansiterm.Context, value interface{}) {
	if ctx != nil {
		ctx.Fprintf(w.TabWriter, "%v\t", value)
	} else {
		fmt.Fprintf(w, "%v\t", value)
	}
}

// PrintStatus writes out the status value in the standard color.
func (w *Wrapper) PrintStatus(status status.Status) {
	w.PrintColor(statusColors[status], status)
}

// CurrentHighlight is the color used to show the current
// controller, user or model in tabular
var CurrentHighlight = ansiterm.Foreground(ansiterm.Green)

// ErrorHighlight is the color used to show error conditions.
var ErrorHighlight = ansiterm.Foreground(ansiterm.Red)

// WarningHighlight is the color used to show warning conditions.
// Generally things that the user should be aware of, but not necessarily
// requiring any user action.
var WarningHighlight = ansiterm.Foreground(ansiterm.Yellow)

// GoodHighlight is used to indicate good or success conditions.
var GoodHighlight = ansiterm.Foreground(ansiterm.Green)

var statusColors = map[status.Status]*ansiterm.Context{
	// good
	status.StatusActive:  GoodHighlight,
	status.StatusIdle:    GoodHighlight,
	status.StatusStarted: GoodHighlight,
	// busy
	status.StatusAllocating:  WarningHighlight,
	status.StatusExecuting:   WarningHighlight,
	status.StatusLost:        WarningHighlight,
	status.StatusMaintenance: WarningHighlight,
	status.StatusPending:     WarningHighlight,
	status.StatusRebooting:   WarningHighlight,
	status.StatusStopped:     WarningHighlight,
	status.StatusUnknown:     WarningHighlight,
	// bad
	status.StatusBlocked: ErrorHighlight,
	status.StatusDown:    ErrorHighlight,
	status.StatusError:   ErrorHighlight,
	status.StatusFailed:  ErrorHighlight,
}
