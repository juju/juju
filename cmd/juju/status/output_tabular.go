// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/juju/ansiterm"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/crossmodel"
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

// FormatTabular writes a tabular summary of machines, applications, and
// units. Any subordinate items are indented by two spaces beneath
// their superior.
func FormatTabular(writer io.Writer, forceColor bool, value interface{}) error {
	const maxVersionWidth = 15
	const ellipsis = "..."
	const truncatedWidth = maxVersionWidth - len(ellipsis)

	fs, valueConverted := value.(formattedStatus)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", fs, value)
	}
	// To format things into columns.
	tw := output.TabWriter(writer)
	if forceColor {
		tw.SetColorCapable(forceColor)
	}
	w := output.Wrapper{tw}
	p := w.Println
	outputHeaders := func(values ...interface{}) {
		p()
		p(values...)
	}

	cloudRegion := fs.Model.Cloud
	if fs.Model.CloudRegion != "" {
		cloudRegion += "/" + fs.Model.CloudRegion
	}

	header := []interface{}{"Model", "Controller", "Cloud/Region", "Version"}
	values := []interface{}{fs.Model.Name, fs.Model.Controller, cloudRegion, fs.Model.Version}
	message := getModelMessage(fs.Model)
	if message != "" {
		header = append(header, "Notes")
		values = append(values, message)
	}

	// The first set of headers don't use outputHeaders because it adds the blank line.
	p(header...)
	p(values...)

	if len(fs.RemoteApplications) > 0 {
		outputHeaders("SAAS name", "Status", "Store", "URL", "Interfaces")
		for _, svcName := range utils.SortStringsNaturally(stringKeysFromMap(fs.RemoteApplications)) {
			svc := fs.RemoteApplications[svcName]
			var store, urlPath string
			url, err := crossmodel.ParseApplicationURL(svc.ApplicationURL)
			if err == nil {
				store = url.Directory
				urlPath = url.Path()
			} else {
				// This is not expected.
				logger.Errorf("invalid application URL %q: %v", svc.ApplicationURL, err)
				store = "unknown"
				urlPath = svc.ApplicationURL
			}
			interfaces := make([]string, len(svc.Endpoints))
			for i, name := range utils.SortStringsNaturally(stringKeysFromMap(svc.Endpoints)) {
				ep := svc.Endpoints[name]
				interfaces[i] = fmt.Sprintf("%s:%s", ep.Interface, name)
			}
			p(svcName, svc.StatusInfo.Current, store, urlPath, strings.Join(interfaces, ", "))
		}
		tw.Flush()
	}

	units := make(map[string]unitStatus)
	metering := false
	relations := newRelationFormatter()
	outputHeaders("App", "Version", "Status", "Scale", "Charm", "Store", "Rev", "OS", "Notes")
	tw.SetColumnAlignRight(3)
	tw.SetColumnAlignRight(6)
	for _, appName := range utils.SortStringsNaturally(stringKeysFromMap(fs.Applications)) {
		app := fs.Applications[appName]
		version := app.Version
		// Don't let a long version push out the version column.
		if len(version) > maxVersionWidth {
			version = version[:truncatedWidth] + ellipsis
		}
		// Notes may well contain other things later.
		notes := ""
		if app.Exposed {
			notes = "exposed"
		}
		w.Print(appName, version)
		w.PrintStatus(app.StatusInfo.Current)
		scale, warn := fs.applicationScale(appName)
		if warn {
			w.PrintColor(output.WarningHighlight, scale)
		} else {
			w.Print(scale)
		}
		p(app.CharmName,
			app.CharmOrigin,
			app.CharmRev,
			app.OS,
			notes)

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

	pUnit := func(name string, u unitStatus, level int) {
		message := u.WorkloadStatusInfo.Message
		agentDoing := agentDoing(u.JujuStatusInfo)
		if agentDoing != "" {
			message = fmt.Sprintf("(%s) %s", agentDoing, message)
		}
		if u.Leader {
			name += "*"
		}
		w.Print(indent("", level*2, name))
		w.PrintStatus(u.WorkloadStatusInfo.Current)
		w.PrintStatus(u.JujuStatusInfo.Current)
		p(
			u.Machine,
			u.PublicAddress,
			strings.Join(u.OpenedPorts, ","),
			message,
		)
	}

	outputHeaders("Unit", "Workload", "Agent", "Machine", "Public address", "Ports", "Message")
	for _, name := range utils.SortStringsNaturally(stringKeysFromMap(units)) {
		u := units[name]
		pUnit(name, u, 0)
		const indentationLevel = 1
		recurseUnits(u, indentationLevel, pUnit)
	}

	if metering {
		outputHeaders("Meter", "Status", "Message")
		for _, name := range utils.SortStringsNaturally(stringKeysFromMap(units)) {
			u := units[name]
			if u.MeterStatus != nil {
				w.Print(name)
				outputColor := fromMeterStatusColor(u.MeterStatus.Color)
				w.PrintColor(outputColor, u.MeterStatus.Color)
				w.PrintColor(outputColor, u.MeterStatus.Message)
				w.Println()
			}
		}
	}

	p()
	printMachines(tw, fs.Machines)

	if relations.len() > 0 {
		outputHeaders("Relation", "Provides", "Consumes", "Type")
		for _, k := range relations.sorted() {
			r := relations.get(k)
			if r != nil {
				p(r.relation, r.application1, r.application2, r.relationType())
			}
		}
	}

	tw.Flush()
	return nil
}

func fromMeterStatusColor(msColor string) *ansiterm.Context {
	switch msColor {
	case "green":
		return output.GoodHighlight
	case "amber":
		return output.WarningHighlight
	case "red":
		return output.ErrorHighlight
	}
	return nil
}

func getModelMessage(model modelStatus) string {
	// Select the most important message about the model (if any).
	switch {
	case model.Status.Message != "":
		return model.Status.Message
	case model.AvailableVersion != "":
		return "upgrade available: " + model.AvailableVersion
	default:
		return ""
	}
}

func printMachines(tw *ansiterm.TabWriter, machines map[string]machineStatus) {
	w := output.Wrapper{tw}
	w.Println("Machine", "State", "DNS", "Inst id", "Series", "AZ")
	for _, name := range utils.SortStringsNaturally(stringKeysFromMap(machines)) {
		printMachine(w, machines[name])
	}
}

func printMachine(w output.Wrapper, m machineStatus) {
	// We want to display availability zone so extract from hardware info".
	hw, err := instance.ParseHardware(m.Hardware)
	if err != nil {
		logger.Warningf("invalid hardware info %s for machine %v", m.Hardware, m)
	}
	az := ""
	if hw.AvailabilityZone != nil {
		az = *hw.AvailabilityZone
	}
	w.Print(m.Id)
	w.PrintStatus(m.JujuStatus.Current)
	w.Println(m.DNSName, m.InstanceId, m.Series, az)
	for _, name := range utils.SortStringsNaturally(stringKeysFromMap(m.Containers)) {
		printMachine(w, m.Containers[name])
	}
}

// FormatMachineTabular writes a tabular summary of machine
func FormatMachineTabular(writer io.Writer, forceColor bool, value interface{}) error {
	fs, valueConverted := value.(formattedMachineStatus)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", fs, value)
	}
	tw := output.TabWriter(writer)
	if forceColor {
		tw.SetColorCapable(forceColor)
	}
	printMachines(tw, fs.Machines)
	tw.Flush()

	return nil
}

// agentDoing returns what hook or action, if any,
// the agent is currently executing.
// The hook name or action is extracted from the agent message.
func agentDoing(agentStatus statusInfoContents) string {
	if agentStatus.Current != status.Executing {
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
