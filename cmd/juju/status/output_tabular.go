// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
)

// FormatTabular returns a tabular summary of machines, services, and
// units. Any subordinate items are indented by two spaces beneath
// their superior.
func FormatTabular(value interface{}) ([]byte, error) {
	fs, valueConverted := value.(formattedStatus)
	if !valueConverted {
		return nil, errors.Errorf("expected value of type %T, got %T", fs, value)
	}
	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)
	p := func(values ...interface{}) {
		for _, v := range values {
			fmt.Fprintf(tw, "%s\t", v)
		}
		fmt.Fprintln(tw)
	}

	units := make(map[string]unitStatus)
	p("[Services]")
	p("NAME\tSTATUS\tEXPOSED\tCHARM")
	for _, svcName := range common.SortStringsNaturally(stringKeysFromMap(fs.Services)) {
		svc := fs.Services[svcName]
		for un, u := range svc.Units {
			units[un] = u
		}
		p(svcName, svc.StatusInfo.Current, fmt.Sprintf("%t", svc.Exposed), svc.Charm)
	}
	tw.Flush()

	pUnit := func(name string, u unitStatus, level int) {
		message := u.WorkloadStatusInfo.Message
		agentDoing := agentDoing(u.AgentStatusInfo)
		if agentDoing != "" {
			message = fmt.Sprintf("(%s) %s", agentDoing, message)
		}
		p(
			indent("", level*2, name),
			u.WorkloadStatusInfo.Current,
			u.AgentStatusInfo.Current,
			u.AgentStatusInfo.Version,
			u.Machine,
			strings.Join(u.OpenedPorts, ","),
			u.PublicAddress,
			message,
		)
	}

	// See if we have new or old data; that determines what data we can display.
	newStatus := false
	for _, u := range units {
		if u.AgentStatusInfo.Current != "" {
			newStatus = true
			break
		}
	}
	var header []string
	if newStatus {
		header = []string{"ID", "WORKLOAD-STATE", "AGENT-STATE", "VERSION", "MACHINE", "PORTS", "PUBLIC-ADDRESS", "MESSAGE"}
	} else {
		header = []string{"ID", "STATE", "VERSION", "MACHINE", "PORTS", "PUBLIC-ADDRESS"}
	}

	p("\n[Units]")
	p(strings.Join(header, "\t"))
	for _, name := range common.SortStringsNaturally(stringKeysFromMap(units)) {
		u := units[name]
		pUnit(name, u, 0)
		const indentationLevel = 1
		recurseUnits(u, indentationLevel, pUnit)
	}
	tw.Flush()

	p("\n[Machines]")
	p("ID\tSTATE\tVERSION\tDNS\tINS-ID\tSERIES\tHARDWARE")
	for _, name := range common.SortStringsNaturally(stringKeysFromMap(fs.Machines)) {
		m := fs.Machines[name]
		p(m.Id, m.AgentState, m.AgentVersion, m.DNSName, m.InstanceId, m.Series, m.Hardware)
	}
	tw.Flush()

	return out.Bytes(), nil
}

// agentDoing returns what hook or action, if any,
// the agent is currently executing.
// The hook name or action is extracted from the agent message.
func agentDoing(status statusInfoContents) string {
	if status.Current != params.StatusExecuting {
		return ""
	}
	// First see if we can determine a hook name.
	var hookNames []string
	for _, h := range hooks.UnitHooks() {
		hookNames = append(hookNames, string(h))
	}
	for _, h := range hooks.RelationHooks() {
		hookNames = append(hookNames, string(h))
	}
	hookExp := regexp.MustCompile(fmt.Sprintf(`running (?P<hook>%s?) hook`, strings.Join(hookNames, "|")))
	match := hookExp.FindStringSubmatch(status.Message)
	if len(match) > 0 {
		return match[1]
	}
	// Now try for an action name.
	actionExp := regexp.MustCompile(`running action (?P<action>.*)`)
	match = actionExp.FindStringSubmatch(status.Message)
	if len(match) > 0 {
		return match[1]
	}
	return ""
}
