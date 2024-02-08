// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package output

import (
	"fmt"
	"io"

	"github.com/juju/ansiterm"
	"github.com/juju/cmd/v4"

	"github.com/juju/juju/core/status"
)

// DefaultFormatters holds the formatters that can be
// specified with the --format flag.
var DefaultFormatters = map[string]cmd.Formatter{
	"yaml": cmd.FormatYaml,
	"json": cmd.FormatJson,
}

// FormatYamlWithColor formats yaml output with color.
func FormatYamlWithColor(w io.Writer, value interface{}) error {
	result, err := marshalYaml(value)
	if err != nil {
		return err
	}

	fmt.Fprint(w, string(result))

	return nil
}

// FormatJsonWithColor formats json output with color.
func FormatJsonWithColor(w io.Writer, val interface{}) error {
	if val == nil {
		return nil
	}

	result, err := marshal(val)
	if err != nil {
		return err
	}

	fmt.Fprintln(w, string(result))

	return err
}

// Writer returns a new writer that appends ansi color codes to the output.
func Writer(writer io.Writer) *ansiterm.Writer {
	return ansiterm.NewWriter(writer)
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

// PrintNoTab writes each value adjacent to each other.
func (w *Wrapper) PrintNoTab(values ...interface{}) {
	for _, v := range values {
		fmt.Fprintf(w, "%v", v)
	}
}

// Printf writes the formatted text followed by a tab.
func (w *Wrapper) Printf(format string, values ...interface{}) {
	fmt.Fprintf(w, format+"\t", values...)
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

// PrintColorNoTab writes the value out in the color context specified.
func (w *Wrapper) PrintColorNoTab(ctx *ansiterm.Context, value interface{}) {
	if ctx != nil {
		ctx.Fprintf(w.TabWriter, "%v", value)
	} else {
		fmt.Fprintf(w, "%v", value)
	}
}

// PrintHeaders writes out many tab separated values in the color context specified.
func (w *Wrapper) PrintHeaders(ctx *ansiterm.Context, values ...interface{}) {
	for i, v := range values {
		if i != len(values)-1 {
			ctx.Fprintf(w, "%v\t", v)
		} else {
			ctx.Fprintf(w, "%v", v)
		}
	}
	fmt.Fprintln(w)
}

// PrintStatus writes out the status value in the standard color.
func (w *Wrapper) PrintStatus(status status.Status) {
	w.PrintColor(statusColors[status], status)
}

// StatusColor returns the status's standard color
func StatusColor(status status.Status) *ansiterm.Context {
	if val, ok := statusColors[status]; ok {
		return val
	}
	return CurrentHighlight
}

// PrintWriter decorates the ansiterm.Writer object.
type PrintWriter struct {
	*ansiterm.Writer
}

// Printf writes each value.
func (w *PrintWriter) Printf(ctx *ansiterm.Context, format string, values ...interface{}) {
	if ctx != nil {
		ctx.Fprintf(w, format, values...) //if ctx != nil {"%v" =format
	} else {
		fmt.Fprintf(w, format, values...)
	}
}

// Println writes each value.
func (w *PrintWriter) Println(ctx *ansiterm.Context, values ...interface{}) {
	for _, v := range values {
		ctx.Fprintf(w, "%v", v)
	}
	fmt.Fprintln(w)
}

// Print empty tab after values
func (w *PrintWriter) Print(values ...interface{}) {
	w.Printf(CurrentHighlight, "%v", values...)
}

// PrintNoTab prints values without a tab delimiter
func (w *PrintWriter) PrintNoTab(values ...interface{}) {
	w.Printf(CurrentHighlight, "%v", values...)
}

// PrintColorNoTab writes the value out in the color context specified.
func (w *PrintWriter) PrintColorNoTab(ctx *ansiterm.Context, value interface{}) {
	w.Printf(ctx, "%v", value)
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

// InfoHighlight is  the color used to indicate important details.
// Generally that might be important to a user but not necessarily that
// obvious.
var InfoHighlight = ansiterm.Foreground(ansiterm.Cyan)

// EmphasisHighlight is used to show accompanying information, which
// might be deemed as important by the user.
var EmphasisHighlight = struct {
	White             *ansiterm.Context
	DefaultBold       *ansiterm.Context
	BoldWhite         *ansiterm.Context
	Gray              *ansiterm.Context
	BoldGray          *ansiterm.Context
	DarkGray          *ansiterm.Context
	BoldDarkGray      *ansiterm.Context
	Magenta           *ansiterm.Context
	BrightMagenta     *ansiterm.Context
	BoldBrightMagenta *ansiterm.Context
	BrightGreen       *ansiterm.Context
}{
	White:             ansiterm.Foreground(ansiterm.White),
	DefaultBold:       ansiterm.Foreground(ansiterm.Default).SetStyle(ansiterm.Bold),
	BoldWhite:         ansiterm.Foreground(ansiterm.White).SetStyle(ansiterm.Bold),
	Gray:              ansiterm.Foreground(ansiterm.Gray),
	BoldGray:          ansiterm.Foreground(ansiterm.Gray).SetStyle(ansiterm.Bold),
	BoldDarkGray:      ansiterm.Foreground(ansiterm.DarkGray).SetStyle(ansiterm.Bold),
	DarkGray:          ansiterm.Foreground(ansiterm.DarkGray),
	Magenta:           ansiterm.Foreground(ansiterm.Magenta),
	BrightMagenta:     ansiterm.Foreground(ansiterm.BrightMagenta),
	BoldBrightMagenta: ansiterm.Foreground(ansiterm.BrightMagenta).SetStyle(ansiterm.Bold),
	BrightGreen:       ansiterm.Foreground(ansiterm.BrightGreen),
}

var statusColors = map[status.Status]*ansiterm.Context{
	// good
	status.Active:    GoodHighlight,
	status.Running:   GoodHighlight,
	status.Idle:      GoodHighlight,
	status.Started:   GoodHighlight,
	status.Executing: GoodHighlight,
	status.Attaching: GoodHighlight,
	status.Attached:  GoodHighlight,
	// busy
	status.Allocating:  WarningHighlight,
	status.Lost:        WarningHighlight,
	status.Maintenance: WarningHighlight,
	status.Pending:     WarningHighlight,
	status.Rebooting:   WarningHighlight,
	status.Stopped:     WarningHighlight,
	status.Unknown:     WarningHighlight,
	status.Detaching:   WarningHighlight,
	status.Detached:    WarningHighlight,
	// bad
	status.Blocked:    ErrorHighlight,
	status.Down:       ErrorHighlight,
	status.Error:      ErrorHighlight,
	status.Failed:     ErrorHighlight,
	status.Terminated: ErrorHighlight,
}
