// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/status"
)

type statusRelation struct {
	application1 string
	application2 string
	relation     string
	subordinate  bool
}

func (s *statusRelation) relationType() string {
	if s.subordinate {
		return "subordinate"
	} else if s.application1 == s.application2 {
		return "peer"
	}
	return "regular"
}

type relationFormatter struct {
	relationIndex set.Strings
	relations     map[string]*statusRelation
}

func newRelationFormatter() *relationFormatter {
	return &relationFormatter{
		relationIndex: set.NewStrings(),
		relations:     make(map[string]*statusRelation),
	}
}

func (r *relationFormatter) len() int {
	return r.relationIndex.Size()
}

func (r *relationFormatter) add(rel1, rel2, relation string, is2SubOf1 bool) {
	rel := []string{rel1, rel2}
	if !is2SubOf1 {
		sort.Sort(sort.StringSlice(rel))
	}
	k := strings.Join(rel, "\t")
	r.relations[k] = &statusRelation{
		application1: rel[0],
		application2: rel[1],
		relation:     relation,
		subordinate:  is2SubOf1,
	}
	r.relationIndex.Add(k)
}

func (r *relationFormatter) sorted() []string {
	return r.relationIndex.SortedValues()
}

func (r *relationFormatter) get(k string) *statusRelation {
	return r.relations[k]
}

func printHelper(tw io.Writer) func(...interface{}) {
	return func(values ...interface{}) {
		for i, v := range values {
			if i != len(values)-1 {
				fmt.Fprintf(tw, "%v\t", v)
			} else {
				fmt.Fprintf(tw, "%v", v)
			}
		}
		fmt.Fprintln(tw)
	}
}

// FormatTabular writes a tabular summary of machines, applications, and
// units. Any subordinate items are indented by two spaces beneath
// their superior.
func FormatTabular(writer io.Writer, value interface{}) error {
	const maxVersionWidth = 7
	const ellipsis = "..."
	const truncatedWidth = maxVersionWidth - len(ellipsis)

	fs, valueConverted := value.(formattedStatus)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", fs, value)
	}
	// To format things into columns.
	tw := output.TabWriter(writer)
	p := printHelper(tw)
	outputHeaders := func(values ...interface{}) {
		p()
		p(values...)
	}

	cloudRegion := fs.Model.Cloud
	if fs.Model.CloudRegion != "" {
		cloudRegion += "/" + fs.Model.CloudRegion
	}

	header := []interface{}{"MODEL", "CONTROLLER", "CLOUD/REGION", "VERSION"}
	values := []interface{}{fs.Model.Name, fs.Model.Controller, cloudRegion, fs.Model.Version}
	message := getModelMessage(fs.Model)
	if message != "" {
		header = append(header, "MESSAGE")
		values = append(values, message)
	}

	// The first set of headers don't use outputHeaders because it adds the blank line.
	p(header...)
	p(values...)

	units := make(map[string]unitStatus)
	metering := false
	relations := newRelationFormatter()
	outputHeaders("APP", "VERSION", "STATUS", "EXPOSED", "ORIGIN", "CHARM", "REV", "OS")
	for _, appName := range utils.SortStringsNaturally(stringKeysFromMap(fs.Applications)) {
		app := fs.Applications[appName]
		version := app.Version
		// Don't let a long version push out the version column.
		if len(version) > maxVersionWidth {
			version = version[:truncatedWidth] + ellipsis
		}
		p(appName,
			version,
			app.StatusInfo.Current,
			fmt.Sprintf("%t", app.Exposed),
			app.CharmOrigin,
			app.CharmName,
			app.CharmRev,
			app.OS)

		for un, u := range app.Units {
			units[un] = u
			if u.MeterStatus != nil {
				metering = true
			}
		}

		// Ensure that we pick a consistent name for peer relations.
		sortedRelTypes := make([]string, 0, len(app.Relations))
		for relType := range app.Relations {
			sortedRelTypes = append(sortedRelTypes, relType)
		}
		sort.Strings(sortedRelTypes)

		subs := set.NewStrings(app.SubordinateTo...)
		for _, relType := range sortedRelTypes {
			for _, related := range app.Relations[relType] {
				relations.add(related, appName, relType, subs.Contains(related))
			}
		}

	}
	if relations.len() > 0 {
		outputHeaders("RELATION", "PROVIDES", "CONSUMES", "TYPE")
		for _, k := range relations.sorted() {
			r := relations.get(k)
			if r != nil {
				p(r.relation, r.application1, r.application2, r.relationType())
			}
		}
	}

	pUnit := func(name string, u unitStatus, level int) {
		message := u.WorkloadStatusInfo.Message
		agentDoing := agentDoing(u.JujuStatusInfo)
		if agentDoing != "" {
			message = fmt.Sprintf("(%s) %s", agentDoing, message)
		}
		p(
			indent("", level*2, name),
			u.WorkloadStatusInfo.Current,
			u.JujuStatusInfo.Current,
			u.Machine,
			u.PublicAddress,
			strings.Join(u.OpenedPorts, ","),
			message,
		)
	}

	outputHeaders("UNIT", "WORKLOAD", "AGENT", "MACHINE", "PUBLIC-ADDRESS", "PORTS", "MESSAGE")
	for _, name := range utils.SortStringsNaturally(stringKeysFromMap(units)) {
		u := units[name]
		pUnit(name, u, 0)
		const indentationLevel = 1
		recurseUnits(u, indentationLevel, pUnit)
	}

	if metering {
		outputHeaders("METER", "STATUS", "MESSAGE")
		for _, name := range utils.SortStringsNaturally(stringKeysFromMap(units)) {
			u := units[name]
			if u.MeterStatus != nil {
				p(name, u.MeterStatus.Color, u.MeterStatus.Message)
			}
		}
	}

	var pMachine func(machineStatus)
	pMachine = func(m machineStatus) {
		// We want to display availability zone so extract from hardware info".
		hw, err := instance.ParseHardware(m.Hardware)
		if err != nil {
			logger.Warningf("invalid hardware info %s for machine %v", m.Hardware, m)
		}
		az := ""
		if hw.AvailabilityZone != nil {
			az = *hw.AvailabilityZone
		}
		p(m.Id, m.JujuStatus.Current, m.DNSName, m.InstanceId, m.Series, az)
		for _, name := range utils.SortStringsNaturally(stringKeysFromMap(m.Containers)) {
			pMachine(m.Containers[name])
		}
	}

	p()
	printMachines(tw, fs.Machines)
	tw.Flush()
	return nil
}

func getModelMessage(model modelStatus) string {
	// Select the most important message about the model (if any).
	switch {
	case model.Migration != "":
		return "migrating: " + model.Migration
	case model.AvailableVersion != "":
		return "upgrade available: " + model.AvailableVersion
	default:
		return ""
	}
}

func printMachines(tw io.Writer, machines map[string]machineStatus) {
	p := printHelper(tw)
	p("MACHINE", "STATE", "DNS", "INS-ID", "SERIES", "AZ")
	for _, name := range utils.SortStringsNaturally(stringKeysFromMap(machines)) {
		printMachine(p, machines[name], "")
	}
}

func printMachine(p func(...interface{}), m machineStatus, prefix string) {
	// We want to display availability zone so extract from hardware info".
	hw, err := instance.ParseHardware(m.Hardware)
	if err != nil {
		logger.Warningf("invalid hardware info %s for machine %v", m.Hardware, m)
	}
	az := ""
	if hw.AvailabilityZone != nil {
		az = *hw.AvailabilityZone
	}
	p(prefix+m.Id, m.JujuStatus.Current, m.DNSName, m.InstanceId, m.Series, az)
	for _, name := range utils.SortStringsNaturally(stringKeysFromMap(m.Containers)) {
		printMachine(p, m.Containers[name], prefix+"  ")
	}
}

// FormatMachineTabular writes a tabular summary of machine
func FormatMachineTabular(writer io.Writer, value interface{}) error {
	fs, valueConverted := value.(formattedMachineStatus)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", fs, value)
	}
	tw := output.TabWriter(writer)
	printMachines(tw, fs.Machines)
	tw.Flush()

	return nil
}

// agentDoing returns what hook or action, if any,
// the agent is currently executing.
// The hook name or action is extracted from the agent message.
func agentDoing(agentStatus statusInfoContents) string {
	if agentStatus.Current != status.StatusExecuting {
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
	match := hookExp.FindStringSubmatch(agentStatus.Message)
	if len(match) > 0 {
		return match[1]
	}
	// Now try for an action name.
	actionExp := regexp.MustCompile(`running action (?P<action>.*)`)
	match = actionExp.FindStringSubmatch(agentStatus.Message)
	if len(match) > 0 {
		return match[1]
	}
	return ""
}
