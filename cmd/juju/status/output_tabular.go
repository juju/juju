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
	"github.com/juju/naturalsort"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/hooks"

	cmdcrossmodel "github.com/juju/juju/cmd/juju/crossmodel"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/instance"
)

const (
	caasModelType       = "caas"
	ellipsis            = "..."
	iaasMaxVersionWidth = 15
	caasMaxVersionWidth = 30
)

// FormatTabular writes a tabular summary of machines, applications, and
// units. Any subordinate items are indented by two spaces beneath
// their superior.
func FormatTabular(writer io.Writer, forceColor bool, value interface{}) error {
	fs, valueConverted := value.(formattedStatus)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", fs, value)
	}

	// To format things into columns.
	tw := output.TabWriter(writer)
	if forceColor {
		tw.SetColorCapable(forceColor)
	}

	cloudRegion := fs.Model.Cloud
	if fs.Model.CloudRegion != "" {
		cloudRegion += "/" + fs.Model.CloudRegion
	}

	// Default table output
	header := []interface{}{"Model", "Controller", "Cloud/Region", "Version"}
	values := []interface{}{fs.Model.Name, fs.Model.Controller, cloudRegion, fs.Model.Version}

	// Optional table output if values exist
	message := getModelMessage(fs.Model)
	if fs.Model.SLA != "" {
		header = append(header, "SLA")
		values = append(values, fs.Model.SLA)
	}
	if cs := fs.Controller; cs != nil && cs.Timestamp != "" {
		header = append(header, "Timestamp")
		values = append(values, cs.Timestamp)
	}
	if message != "" {
		header = append(header, "Notes")
		values = append(values, message)
	}

	// The first set of headers don't use outputHeaders because it adds the blank line.
	w := startSection(tw, true, header...)
	w.Println(values...)

	if len(fs.RemoteApplications) > 0 {
		printRemoteApplications(tw, fs.RemoteApplications)
	}

	if len(fs.Applications) > 0 {
		printApplications(tw, fs)
	}

	if fs.Model.Type != caasModelType && len(fs.Machines) > 0 {
		printMachines(tw, false, fs.Machines)
	}

	if err := printOffers(tw, fs.Offers); err != nil {
		w.Println(err.Error())
	}

	if len(fs.Relations) > 0 {
		printRelations(tw, fs.Relations)
	}

	endSection(tw)
	return nil
}

func startSection(tw *ansiterm.TabWriter, top bool, headers ...interface{}) output.Wrapper {
	w := output.Wrapper{tw}
	if !top {
		w.Println()
	}
	w.Println(headers...)
	return w
}

func endSection(tw *ansiterm.TabWriter) {
	tw.Flush()
}

func printApplications(tw *ansiterm.TabWriter, fs formattedStatus) {
	maxVersionWidth := iaasMaxVersionWidth
	if fs.Model.Type == caasModelType {
		maxVersionWidth = caasMaxVersionWidth
	}
	truncatedWidth := maxVersionWidth - len(ellipsis)

	metering := fs.Model.MeterStatus != nil
	units := make(map[string]unitStatus)
	var w output.Wrapper
	if fs.Model.Type == caasModelType {
		w = startSection(tw, false, "App", "Version", "Status", "Scale", "Charm", "Store", "Rev", "OS", "Address", "Notes")
	} else {
		w = startSection(tw, false, "App", "Version", "Status", "Scale", "Charm", "Store", "Rev", "OS", "Notes")
	}
	tw.SetColumnAlignRight(3)
	tw.SetColumnAlignRight(6)
	for _, appName := range naturalsort.Sort(stringKeysFromMap(fs.Applications)) {
		app := fs.Applications[appName]
		version := app.Version
		// CAAS versions may have repo prefix we don't care about.
		if fs.Model.Type == caasModelType {
			parts := strings.Split(version, "/")
			if len(parts) == 2 {
				version = parts[1]
			}
		}
		// Don't let a long version push out the version column.
		if len(version) > maxVersionWidth {
			version = version[:truncatedWidth] + ellipsis
		}
		// Notes may well contain other things later.
		notes := ""
		if app.Exposed {
			notes = "exposed"
		}
		// Expose any operator messages.
		if fs.Model.Type == caasModelType {
			if app.StatusInfo.Message != "" {
				notes = app.StatusInfo.Message
			}
		}
		w.Print(appName, version)
		w.PrintStatus(app.StatusInfo.Current)
		scale, warn := fs.applicationScale(appName)
		if warn {
			w.PrintColor(output.WarningHighlight, scale)
		} else {
			w.Print(scale)
		}

		w.Print(app.CharmName,
			app.CharmOrigin,
			app.CharmRev,
			app.OS)
		if fs.Model.Type == caasModelType {
			w.Print(app.Address)
		}

		w.Println(notes)
		for un, u := range app.Units {
			units[un] = u
			if u.MeterStatus != nil {
				metering = true
			}
		}
	}
	endSection(tw)

	pUnit := func(name string, u unitStatus, level int) {
		message := u.WorkloadStatusInfo.Message
		// If we're still allocating and there's a message, show that.
		if u.JujuStatusInfo.Current == status.Allocating && message == "" {
			message = u.JujuStatusInfo.Message
		}
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
		if fs.Model.Type == caasModelType {
			w.Println(
				u.Address,
				strings.Join(u.OpenedPorts, ","),
				message,
			)
			return
		}
		w.Println(
			u.Machine,
			u.PublicAddress,
			strings.Join(u.OpenedPorts, ","),
			message,
		)
	}

	if len(units) > 0 {
		if fs.Model.Type == caasModelType {
			startSection(tw, false, "Unit", "Workload", "Agent", "Address", "Ports", "Message")
		} else {
			startSection(tw, false, "Unit", "Workload", "Agent", "Machine", "Public address", "Ports", "Message")
		}
		for _, name := range naturalsort.Sort(stringKeysFromMap(units)) {
			u := units[name]
			pUnit(name, u, 0)
			const indentationLevel = 1
			recurseUnits(u, indentationLevel, pUnit)
		}
		endSection(tw)
	}

	if !metering {
		return
	}

	startSection(tw, false, "Entity", "Meter status", "Message")
	if fs.Model.MeterStatus != nil {
		w.Print("model")
		outputColor := fromMeterStatusColor(fs.Model.MeterStatus.Color)
		w.PrintColor(outputColor, fs.Model.MeterStatus.Color)
		w.PrintColor(outputColor, fs.Model.MeterStatus.Message)
		w.Println()
	}
	for _, name := range naturalsort.Sort(stringKeysFromMap(units)) {
		u := units[name]
		if u.MeterStatus != nil {
			w.Print(name)
			outputColor := fromMeterStatusColor(u.MeterStatus.Color)
			w.PrintColor(outputColor, u.MeterStatus.Color)
			w.PrintColor(outputColor, u.MeterStatus.Message)
			w.Println()
		}
	}
	endSection(tw)
}

func printRemoteApplications(tw *ansiterm.TabWriter, remoteApplications map[string]remoteApplicationStatus) {
	w := startSection(tw, false, "SAAS", "Status", "Store", "URL")
	for _, appName := range naturalsort.Sort(stringKeysFromMap(remoteApplications)) {
		app := remoteApplications[appName]
		var store, urlPath string
		url, err := crossmodel.ParseOfferURL(app.OfferURL)
		if err == nil {
			store = url.Source
			url.Source = ""
			urlPath = url.Path()
			if store == "" {
				store = "local"
			}
		} else {
			// This is not expected.
			logger.Errorf("invalid offer URL %q: %v", app.OfferURL, err)
			store = "unknown"
			urlPath = app.OfferURL
		}
		w.Print(appName)
		w.PrintStatus(app.StatusInfo.Current)
		w.Println(store, urlPath)
	}
	endSection(tw)
}

func printRelations(tw *ansiterm.TabWriter, relations []relationStatus) {
	sort.Slice(relations, func(i, j int) bool {
		a, b := relations[i], relations[j]
		if a.Provider == b.Provider {
			return a.Requirer < b.Requirer
		}
		return a.Provider < b.Provider
	})

	w := startSection(tw, false, "Relation provider", "Requirer", "Interface", "Type", "Message")

	for _, r := range relations {
		w.Print(r.Provider, r.Requirer, r.Interface, r.Type)
		if r.Status != string(relation.Joined) {
			w.PrintColor(cmdcrossmodel.RelationStatusColor(relation.Status(r.Status)), r.Status)
			if r.Message != "" {
				w.Print(" - " + r.Message)
			}
		}
		w.Println()
	}
	endSection(tw)
}

type offerItems []offerStatus

// printOffers prints a tabular summary of the offers.
func printOffers(tw *ansiterm.TabWriter, offers map[string]offerStatus) error {
	if len(offers) == 0 {
		return nil
	}
	w := startSection(tw, false, "Offer", "Application", "Charm", "Rev", "Connected", "Endpoint", "Interface", "Role")
	for _, offerName := range naturalsort.Sort(stringKeysFromMap(offers)) {
		offer := offers[offerName]
		// Sort endpoints alphabetically.
		endpoints := []string{}
		for endpoint := range offer.Endpoints {
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
					fmt.Sprintf("%v/%v", offer.ActiveConnectedCount, offer.TotalConnectedCount),
					endpointName, endpoint.Interface, endpoint.Role)
				continue
			}
			// Subsequent lines only need to display endpoint information.
			// This will display less noise.
			w.Println("", "", "", "", endpointName, endpoint.Interface, endpoint.Role)
		}
	}
	endSection(tw)
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

func printMachines(tw *ansiterm.TabWriter, standAlone bool, machines map[string]machineStatus) {
	w := startSection(tw, standAlone, "Machine", "State", "DNS", "Inst id", "Series", "AZ", "Message")
	for _, name := range naturalsort.Sort(stringKeysFromMap(machines)) {
		printMachine(w, machines[name])
	}
	endSection(tw)
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
	for _, name := range naturalsort.Sort(stringKeysFromMap(m.Containers)) {
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
	printMachines(tw, true, fs.Machines)
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
