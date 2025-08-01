// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"bytes"
	"fmt"
	"io"

	"github.com/juju/ansiterm"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/internal/naturalsort"
)

// FormatOneline writes a brief list of units and their subordinates.
// Subordinates will be indented 2 spaces and listed under their
// superiors. This format works with version 2 of the CLI.
func FormatOneline(writer io.Writer, forceColor bool, value interface{}) error {
	return formatOneline(writer, forceColor, value, func(out io.Writer, format, uName string, u unitStatus, level int) {
		agentColored := colorVal(output.EmphasisHighlight.DefaultBold, "agent")
		statusInfoColored := colorVal(output.StatusColor(u.JujuStatusInfo.Current), u.JujuStatusInfo.Current)
		workloadColored := colorVal(output.EmphasisHighlight.DefaultBold, "workload")
		workloadStatusInfoColored := colorVal(output.StatusColor(u.JujuStatusInfo.Current), u.WorkloadStatusInfo.Current)
		publicAddressColored := colorVal(output.InfoHighlight, u.PublicAddress)

		fPrintf := func(o io.Writer, format string, uName, pAddress, status interface{}) {
			fmt.Fprintf(o, format, uName, pAddress, status)
		}

		if forceColor {
			statusColored := fmt.Sprintf(
				"%s:%s, %s:%s", agentColored, statusInfoColored, workloadColored, workloadStatusInfoColored)
			fPrintf(out, format, uName, publicAddressColored, statusColored)
		} else {
			status := fmt.Sprintf(
				"agent:%s, workload:%s",
				u.JujuStatusInfo.Current,
				u.WorkloadStatusInfo.Current,
			)
			fmt.Fprintf(out, format,
				uName,
				u.PublicAddress,
				status,
			)
		}
	})
}

type onelinePrintf func(out io.Writer, format, uName string, u unitStatus, level int)

func formatOneline(writer io.Writer, forceColor bool, value interface{}, printf onelinePrintf) error {
	fs, valueConverted := value.(formattedStatus)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", fs, value)
	}

	pw := &output.PrintWriter{Writer: output.Writer(writer)}
	pw.SetColorCapable(forceColor)
	pprint := func(uName string, u unitStatus, level int) {
		format := indent("", level*2, "- %s: %s (%v)")
		if forceColor {
			printf(writer, format, colorVal(output.GoodHighlight, uName), u, level)
		} else {
			printf(writer, format, uName, u, level)
		}
		if len(u.OpenedPorts) > 0 {
			fmt.Fprintf(pw, " ")
			printPorts(pw, u.OpenedPorts)
		}
		fmt.Fprintln(pw)
	}
	for _, svcName := range naturalsort.Sort(stringKeysFromMap(fs.Applications)) {
		svc := fs.Applications[svcName]
		for _, uName := range naturalsort.Sort(stringKeysFromMap(svc.Units)) {
			unit := svc.Units[uName]
			pprint(uName, unit, 0)
			recurseUnits(unit, 1, pprint)
		}
	}

	return nil
}

// colorVal appends ansi color codes to the given value
func colorVal(ctx *ansiterm.Context, val interface{}) string {
	buff := &bytes.Buffer{}
	coloredWriter := output.Writer(buff)
	coloredWriter.SetColorCapable(true)
	ctx.Fprint(coloredWriter, val)
	return buff.String()
}
