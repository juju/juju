// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"bytes"
	"fmt"
	"github.com/juju/ansiterm"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/status"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/naturalsort"
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

// FormatOnelineWithColor appends ansi color codes to a brief list of units and their subordinates.
// Subordinates will be indented 2 spaces and listed under their
// superiors. This format works with version 2 of the CLI.
func FormatOnelineWithColor(writer io.Writer, value interface{}) error {
	return formatOnelineWithColor(writer, value, func(out io.Writer, format, uName string, u unitStatus, level int) {
		status := fmt.Sprintf(
			"%s:%s, %s:%s",
			colorVal(output.EmphasisHighlight.DefaultBold, "agent"),
			colorStatus(u.JujuStatusInfo.Current),
			colorVal(output.EmphasisHighlight.DefaultBold, "workload"),
			colorStatus(u.WorkloadStatusInfo.Current),
		)
		fmt.Fprintf(out, format,
			uName,
			colorVal(output.InfoHighlight, u.PublicAddress),
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

func formatOnelineWithColor(writer io.Writer, value interface{}, printf onelinePrintf) error {
	fs, valueConverted := value.(formattedStatus)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", fs, value)
	}

	pprint := func(uName string, u unitStatus, level int) {
		var fmtPorts string
		if len(u.OpenedPorts) > 0 {
			fmtPorts = fmt.Sprintf(" %s", colorPorts(u.OpenedPorts))
		}
		format := indent("\n", level*2, "- %s: %s (%v)"+fmtPorts)
		printf(writer, format, colorVal(output.GoodHighlight, uName), u, level)
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

func colorStatus(stat status.Status) string {
	return colorVal(output.StatusColor(stat), stat)
}

func colorPorts(ps []string) string {
	buff := &bytes.Buffer{}
	sorted := append([]string(nil), ps...)
	sort.Strings(sorted)

	protocols := map[string]*protocol{}
	proto := func(p string) *protocol {
		v, ok := protocols[p]
		if !ok {
			v = &protocol{
				group:      map[string]string{},
				groups:     map[string][]string{},
				grouped:    map[string]bool{},
				components: map[string][]string{},
			}
			protocols[p] = v
		}
		return v
	}

	for _, port := range sorted {
		split := strings.Split(port, "/")
		protocolId := ""
		if len(split) == 1 {
			protocolId = split[0]
		} else {
			protocolId = strings.Join(append([]string{""}, split[1:]...), "/")
		}
		protocol := proto(protocolId)
		protocol.components[port] = split
		if len(split) == 1 {
			continue
		}
		n, err := strconv.Atoi(split[0])
		if err != nil || n <= 1 {
			continue
		}
		prev := strings.Join(append([]string{strconv.Itoa(n - 1)}, split[1:]...), "/")
		if _, ok := protocol.components[prev]; !ok {
			continue
		}
		groupName := protocol.group[prev]
		if groupName == "" {
			groupName = prev
		}
		protocol.group[port] = groupName
		protocol.groups[groupName] = append(protocol.groups[groupName], port)
		protocol.grouped[port] = true
	}

	protocolKeys := []string{}
	for k := range protocols {
		protocolKeys = append(protocolKeys, k)
	}
	sort.Strings(protocolKeys)

	hasOutput := false
	for _, pk := range protocolKeys {
		protocol := protocols[pk]

		portKeys := []string{}
		for k := range protocol.components {
			portKeys = append(portKeys, k)
		}
		sort.Sort(sortablePorts(portKeys))

		hasPrev := false
		for _, port := range portKeys {
			if protocol.grouped[port] {
				continue
			}
			if hasOutput {
				hasOutput = false
				buff.WriteString(" ")
			}
			if hasPrev {
				buff.WriteString(",")
			}
			hasPrev = true
			split := protocol.components[port]
			group := protocol.groups[port]
			// color grouped ports.
			if len(group) > 0 {
				last := group[len(group)-1]
				lastSplit := protocol.components[last]
				portRange := fmt.Sprintf("%s-%s", split[0], lastSplit[0])
				buff.WriteString(colorVal(output.EmphasisHighlight.BrightMagenta, portRange))
				continue
			}
			// color single port with protocol.
			if len(split) > 1 {
				buff.WriteString(colorVal(output.EmphasisHighlight.BrightMagenta, split[0]))
				continue
			}
			// Everything else.
			break
		}
		if hasPrev {
			hasOutput = true
			if _, err := strconv.Atoi(pk); err == nil {
				buff.WriteString(colorVal(output.EmphasisHighlight.BrightMagenta, pk))
			} else {
				buff.WriteString(pk)
			}
		}
	}

	buff.WriteString("")
	return buff.String()
}

//colorVal appends ansi color codes to the given value
func colorVal(ctx *ansiterm.Context, val interface{}) string {
	buff := &bytes.Buffer{}
	coloredWriter := ansiterm.NewWriter(buff)
	coloredWriter.SetColorCapable(true)

	ctx.Fprintf(coloredWriter, "%v", val)
	str := buff.String()
	buff.Reset()
	return str
}
