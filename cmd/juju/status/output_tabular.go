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
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/status"
)

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

	metering := fs.Model.MeterStatus != nil

	header := []interface{}{"Model", "Controller", "Cloud/Region", "Version"}
	values := []interface{}{fs.Model.Name, fs.Model.Controller, cloudRegion, fs.Model.Version}
	message := getModelMessage(fs.Model)
	if message != "" {
		header = append(header, "Notes")
		values = append(values, message)
	}
	if fs.Model.SLA != "" {
		header = append(header, "SLA")
		values = append(values, fs.Model.SLA)
	}

	// The first set of headers don't use outputHeaders because it adds the blank line.
	p(header...)
	p(values...)

	if len(fs.RemoteApplications) > 0 {
		outputHeaders("SAAS", "Status", "Store", "URL")
		for _, appName := range utils.SortStringsNaturally(stringKeysFromMap(fs.RemoteApplications)) {
			app := fs.RemoteApplications[appName]
			var store, urlPath string
			url, err := crossmodel.ParseApplicationURL(app.ApplicationURL)
			if err == nil {
				store = url.Source
				url.Source = ""
				urlPath = url.Path()
				if store == "" {
					store = "local"
				}
			} else {
				// This is not expected.
				logger.Errorf("invalid application URL %q: %v", app.ApplicationURL, err)
				store = "unknown"
				urlPath = app.ApplicationURL
			}
			p(appName, app.StatusInfo.Current, store, urlPath)
		}
		tw.Flush()
	}

	units := make(map[string]unitStatus)
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
		outputHeaders("Entity", "Meter status", "Message")
		if fs.Model.MeterStatus != nil {
			w.Print("model")
			outputColor := fromMeterStatusColor(fs.Model.MeterStatus.Color)
			w.PrintColor(outputColor, fs.Model.MeterStatus.Color)
			w.PrintColor(outputColor, fs.Model.MeterStatus.Message)
			w.Println()
		}
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

	if err := printOffers(tw, fs.Offers); err != nil {
		w.Println(err.Error())
	}

	if len(fs.Relations) > 0 {
		sort.Slice(fs.Relations, func(i, j int) bool {
			a, b := fs.Relations[i], fs.Relations[j]
			if a.Provider == b.Provider {
				return a.Requirer < b.Requirer
			}
			return a.Provider < b.Provider
		})
		outputHeaders("Relation provider", "Requirer", "Interface", "Type")
		for _, r := range fs.Relations {
			p(r.Provider, r.Requirer, r.Interface, r.Type)
		}
	}

	tw.Flush()
	return nil
}

type offerItems []offerStatus

// printOffers prints a tabular summary of the offers.
func printOffers(tw *ansiterm.TabWriter, offers map[string]offerStatus) error {
	if len(offers) == 0 {
		return nil
	}
	w := output.Wrapper{tw}
	w.Println()

	w.Println("Offer", "Application", "Charm", "Rev", "Connected", "Endpoint", "Interface", "Role")
	for _, offerName := range utils.SortStringsNaturally(stringKeysFromMap(offers)) {
		offer := offers[offerName]
		// Sort endpoints alphabetically.
		endpoints := []string{}
		for endpoint, _ := range offer.Endpoints {
			endpoints = append(endpoints, endpoint)
		}
		sort.Strings(endpoints)

		for i, endpointName := range endpoints {

			endpoint := offer.Endpoints[endpointName]
			if i == 0 {
				// As there is some information about offer and its endpoints,
				// only display offer information once when the first endpoint is displayed.
				curl, err := charm.ParseURL(offer.CharmURL)
				if err != nil {
					return errors.Trace(err)
				}
				w.Println(offerName, offer.ApplicationName, curl.Name, fmt.Sprint(curl.Revision),
					fmt.Sprint(offer.ConnectedCount), endpointName, endpoint.Interface, endpoint.Role)
				continue
			}
			// Subsequent lines only need to display endpoint information.
			// This will display less noise.
			w.Println("", "", "", "", endpointName, endpoint.Interface, endpoint.Role)
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
	w.Println("Machine", "State", "DNS", "Inst id", "Series", "AZ", "Message")
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
	w.Println(m.DNSName, m.InstanceId, m.Series, az, m.MachineStatus.Message)
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
