// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/cmd/juju/common"
)

// FormatOneline returns a brief list of units and their subordinates.
// Subordinates will be indented 2 spaces and listed under their
// superiors.
func FormatOneline(value interface{}) ([]byte, error) {
	return formatOneline(value, func(out *bytes.Buffer, format, uName string, u unitStatus, level int) {
		fmt.Fprintf(out, format,
			uName,
			u.PublicAddress,
			u.AgentState,
		)
	})
}

// FormatOnelineV2 returns a brief list of units and their subordinates.
// Subordinates will be indented 2 spaces and listed under their
// superiors. This format works with version 2 of the CLI.
func FormatOnelineV2(value interface{}) ([]byte, error) {
	return formatOneline(value, func(out *bytes.Buffer, format, uName string, u unitStatus, level int) {
		status := fmt.Sprintf(
			"agent:%s, workload:%s",
			u.AgentStatusInfo.Current,
			u.WorkloadStatusInfo.Current,
		)
		fmt.Fprintf(out, format,
			uName,
			u.PublicAddress,
			status,
		)
	})
}

type onelinePrintf func(out *bytes.Buffer, format, uName string, u unitStatus, level int)

func formatOneline(value interface{}, printf onelinePrintf) ([]byte, error) {
	fs, valueConverted := value.(formattedStatus)
	if !valueConverted {
		return nil, errors.Errorf("expected value of type %T, got %T", fs, value)
	}
	var out bytes.Buffer

	pprint := func(uName string, u unitStatus, level int) {
		var fmtPorts string
		if len(u.OpenedPorts) > 0 {
			fmtPorts = fmt.Sprintf(" %s", strings.Join(u.OpenedPorts, ", "))
		}
		format := indent("\n", level*2, "- %s: %s (%v)"+fmtPorts)
		printf(&out, format, uName, u, level)
	}

	for _, svcName := range common.SortStringsNaturally(stringKeysFromMap(fs.Services)) {
		svc := fs.Services[svcName]
		for _, uName := range common.SortStringsNaturally(stringKeysFromMap(svc.Units)) {
			unit := svc.Units[uName]
			pprint(uName, unit, 0)
			recurseUnits(unit, 1, pprint)
		}
	}

	return out.Bytes(), nil
}
