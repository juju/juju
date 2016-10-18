// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"io"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

// FormatOneline writes a brief list of units and their subordinates.
// Subordinates will be indented 2 spaces and listed under their
// superiors. This format works with version 2 of the CLI.
func FormatOneline(writer io.Writer, value interface{}) error {
	return formatOneline(writer, value, func(out io.Writer, format, uName string, u unitStatus, level int) {
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
	})
}

type onelinePrintf func(out io.Writer, format, uName string, u unitStatus, level int)

func formatOneline(writer io.Writer, value interface{}, printf onelinePrintf) error {
	fs, valueConverted := value.(formattedStatus)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", fs, value)
	}

	pprint := func(uName string, u unitStatus, level int) {
		var fmtPorts string
		if len(u.OpenedPorts) > 0 {
			fmtPorts = fmt.Sprintf(" %s", strings.Join(u.OpenedPorts, ", "))
		}
		format := indent("\n", level*2, "- %s: %s (%v)"+fmtPorts)
		printf(writer, format, uName, u, level)
	}

	for _, svcName := range utils.SortStringsNaturally(stringKeysFromMap(fs.Applications)) {
		svc := fs.Applications[svcName]
		for _, uName := range utils.SortStringsNaturally(stringKeysFromMap(svc.Units)) {
			unit := svc.Units[uName]
			pprint(uName, unit, 0)
			recurseUnits(unit, 1, pprint)
		}
	}

	return nil
}
