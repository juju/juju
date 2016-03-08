// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/instance"
)

type statusRelation struct {
	service1    string
	service2    string
	relation    string
	subordinate bool
}

func (s *statusRelation) relationType() string {
	if s.subordinate {
		return "subordinate"
	} else if s.service1 == s.service2 {
		return "peer"
	}
	return "regular"
}

type relationFormatter struct {
	relationIndex []string
	relations     map[string]*statusRelation
}

func newRelationFormatter() *relationFormatter {
	return &relationFormatter{
		relations: make(map[string]*statusRelation),
	}
}

func (r *relationFormatter) len() int {
	return len(r.relationIndex)
}

func (r *relationFormatter) add(rel1, rel2, relation string, is2SubOf1 bool) {
	rel := []string{rel1, rel2}
	if !is2SubOf1 {
		sort.Sort(sort.StringSlice(rel))
	}
	k := strings.Join(rel, "\t")
	r.relations[k] = &statusRelation{
		service1:    rel[0],
		service2:    rel[1],
		relation:    relation,
		subordinate: is2SubOf1,
	}
	r.relationIndex = append(r.relationIndex, k)
}

func (r *relationFormatter) sorted() []string {
	sort.Sort(sort.StringSlice(r.relationIndex))
	return r.relationIndex
}

func (r *relationFormatter) get(k string) *statusRelation {
	return r.relations[k]
}

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

	if envStatus := fs.ModelStatus; envStatus != nil {
		p("[Model]")
		if envStatus.AvailableVersion != "" {
			p("UPGRADE-AVAILABLE")
			p(envStatus.AvailableVersion)
		}
		p()
		tw.Flush()
	}

	units := make(map[string]unitStatus)
	relations := newRelationFormatter()
	p("[Services]")
	p("NAME\tSTATUS\tEXPOSED\tCHARM")
	for _, svcName := range common.SortStringsNaturally(stringKeysFromMap(fs.Services)) {
		svc := fs.Services[svcName]
		for un, u := range svc.Units {
			units[un] = u
		}

		subs := set.NewStrings(svc.SubordinateTo...)
		p(svcName, svc.StatusInfo.Current, fmt.Sprintf("%t", svc.Exposed), svc.Charm)
		for relType, relatedUnits := range svc.Relations {
			for _, related := range relatedUnits {
				relations.add(related, svcName, relType, subs.Contains(related))
			}
		}

	}
	if relations.len() > 0 {
		p()
		p("[Relations]")
		p("SERVICE1\tSERVICE2\tRELATION\tTYPE")
		for _, k := range relations.sorted() {
			r := relations.get(k)
			if r != nil {
				p(r.service1, r.service2, r.relation, r.relationType())
			}
		}
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

	header := []string{"ID", "WORKLOAD-STATE", "AGENT-STATE", "VERSION", "MACHINE", "PORTS", "PUBLIC-ADDRESS", "MESSAGE"}

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
	p("ID\tSTATE\tDNS\tINS-ID\tSERIES\tAZ")
	for _, name := range common.SortStringsNaturally(stringKeysFromMap(fs.Machines)) {
		m := fs.Machines[name]
		// We want to display availability zone so extract from hardware info".
		hw, err := instance.ParseHardware(m.Hardware)
		if err != nil {
			logger.Warningf("invalid hardware info %s for machine %v", m.Hardware, m)
		}
		az := ""
		if hw.AvailabilityZone != nil {
			az = *hw.AvailabilityZone
		}
		p(m.Id, m.AgentState, m.DNSName, m.InstanceId, m.Series, az)
	}
	tw.Flush()
	return out.Bytes(), nil
}

// FormatMachineTabular returns a tabular summary of machine
func FormatMachineTabular(value interface{}) ([]byte, error) {
	fs, valueConverted := value.(formattedMachineStatus)
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

	p("\n[Machines]")
	p("ID\tSTATE\tDNS\tINS-ID\tSERIES\tAZ")
	for _, name := range common.SortStringsNaturally(stringKeysFromMap(fs.Machines)) {
		m := fs.Machines[name]
		// We want to display availability zone so extract from hardware info".
		hw, err := instance.ParseHardware(m.Hardware)
		if err != nil {
			logger.Warningf("invalid hardware info %s for machine %v", m.Hardware, m)
		}
		az := ""
		if hw.AvailabilityZone != nil {
			az = *hw.AvailabilityZone
		}
		p(m.Id, m.AgentState, m.DNSName, m.InstanceId, m.Series, az)
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
