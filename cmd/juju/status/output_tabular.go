// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/distribution/reference"
	"github.com/juju/ansiterm"
	"github.com/juju/errors"

	cmdcrossmodel "github.com/juju/juju/cmd/juju/crossmodel"
	"github.com/juju/juju/cmd/juju/storage"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/output"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/naturalsort"
)

const (
	caasModelType       = "caas"
	ellipsis            = "..."
	iaasMaxVersionWidth = 15
	caasMaxVersionWidth = 30
	maxMessageLength    = 120
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
	values := []interface{}{fs.Model.Name, fs.Model.Controller, cloudRegion}
	// Optional table output if values exist
	message := getModelMessage(fs.Model)
	if cs := fs.Controller; cs != nil && cs.Timestamp != "" {
		header = append(header, "Timestamp")
		values = append(values, cs.Timestamp)
	}
	if message != "" {
		header = append(header, "Notes")
		values = append(values, truncateMessage(message))
	}
	// The first set of headers don't use outputHeaders because it adds the blank line.
	w := startSection(tw, true, header...)
	versionPos := indexOf("Version", header)
	w.Print(values[:versionPos]...)
	if fs.Model.Version != "" {
		modelVersionNum, err := semversion.Parse(fs.Model.Version)
		if err == nil && jujuversion.Current.Compare(modelVersionNum) > 0 {
			w.PrintColor(output.WarningHighlight, fs.Model.Version)
		} else {
			w.Print(fs.Model.Version)
		}
	}

	w.Println(values[versionPos:]...)

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

	if fs.Storage != nil {
		_ = storage.FormatStorageListForStatusTabular(tw, *fs.Storage)
	}

	endSection(tw)
	return nil
}

func startSection(tw *ansiterm.TabWriter, top bool, headers ...interface{}) *output.Wrapper {
	w := &output.Wrapper{TabWriter: tw}
	if !top {
		w.Println()
	}
	w.PrintHeaders(output.EmphasisHighlight.DefaultBold, headers...)

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

	units := make(map[string]unitStatus)
	var w *output.Wrapper
	if fs.Model.Type == caasModelType {
		w = startSection(tw, false, "App", "Version", "Status", "Scale", "Charm", "Channel", "Rev", "Address", "Exposed", "Message")
	} else {
		w = startSection(tw, false, "App", "Version", "Status", "Scale", "Charm", "Channel", "Rev", "Exposed", "Message")
	}
	tw.SetColumnAlignRight(3)
	tw.SetColumnAlignRight(6)
	for _, appName := range naturalsort.Sort(stringKeysFromMap(fs.Applications)) {
		app := fs.Applications[appName]
		// Workload version might be multi-line; we only want the first line for tabular.
		version := strings.Split(app.Version, "\n")[0]
		// CAAS versions may have repo prefix we don't care about.
		if fs.Model.Type == caasModelType && version != "" {
			ref, err := reference.ParseNamed(version)
			if err != nil {
				if err != reference.ErrNameNotCanonical {
					version = ellipsis
				}
			} else {
				version = reference.Path(ref)
				if withTag, ok := ref.(reference.NamedTagged); ok {
					version += ":" + withTag.Tag()
				}
				if withDigest, ok := ref.(reference.Digested); ok {
					digest := withDigest.Digest().Encoded()
					if len(digest) > 7 {
						digest = digest[:7]
					}
					version += "@" + digest
				}
			}
			parts := strings.Split(version, "/")
			// Charms deployed from the rocks or jujucharm repos have the
			// image path set to <repo-url>/<user>/<charm_name>/<resource_name>. Charm name
			// is somewhat redundant and takes up value space. So we'll replace
			// with "res:" to indicate the named resource comes from the store repo.
			fromJujuRepo := strings.Contains(app.Version, "jujucharms.") || strings.Contains(app.Version, "rocks.")
			if fromJujuRepo && len(parts) > 2 && (len(version) > maxVersionWidth || parts[1] == app.Charm) {
				prefix := ""
				switch app.CharmOrigin {
				case "charmhub", "charmstore":
					prefix = "res:"
				}
				if prefix != "" {
					parts[2] = prefix + parts[2]
					version = strings.Join(parts[2:], "/")
				}
			}
			// For qualified images, if they are too long, we still
			// want to see the core image name, but we can compromise
			// on the namespace part and replace that with "...".
			parts = strings.Split(version, "/")
			if len(parts) > 1 && len(version) > maxVersionWidth {
				parts[0] = ellipsis
				version = strings.Join(parts, "/")
			}
		}
		// Don't let a long version push out the version column.
		if len(version) > maxVersionWidth {
			version = version[:truncatedWidth] + ellipsis
		}
		w.Print(appName, version)
		w.PrintStatus(app.StatusInfo.Current)
		scale, warn := fs.applicationScale(appName)
		if warn {
			w.PrintColor(output.WarningHighlight, scale)
		} else {
			w.Print(scale)
		}

		w.Print(app.CharmName, app.CharmChannel)
		if app.CanUpgradeTo != "" {
			w.PrintColor(output.WarningHighlight, app.CharmRev)
		} else {
			w.Print(app.CharmRev)
		}
		if fs.Model.Type == caasModelType {
			w.PrintColor(output.InfoHighlight, app.Address)
		}

		if app.Exposed {
			w.PrintColor(output.GoodHighlight, "yes")
		} else {
			w.Print("no")
		}

		w.PrintColorNoTab(output.EmphasisHighlight.Gray, truncateMessage(app.StatusInfo.Message))
		w.Println()
		for un, u := range app.Units {
			units[un] = u
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
			w.PrintColor(output.InfoHighlight, u.Address)
			printPorts(w, u.OpenedPorts)
			w.PrintColorNoTab(output.EmphasisHighlight.Gray, truncateMessage(message))
			w.Println()
			return
		}
		w.Print(u.Machine)
		w.PrintColor(output.InfoHighlight, u.PublicAddress)
		printPorts(w, u.OpenedPorts)
		w.PrintColorNoTab(output.EmphasisHighlight.Gray, truncateMessage(message))
		w.Println()
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

	endSection(tw)
}

type protocol struct {
	group      map[string]string
	groups     map[string][]string
	grouped    map[string]bool
	components map[string][]string
}

type sortablePorts []string

func (s sortablePorts) Len() int {
	return len(s)
}

func (s sortablePorts) Less(i, j int) bool {
	if len(s[i]) < len(s[j]) {
		return true
	}
	if len(s[i]) > len(s[j]) {
		return false
	}
	return s[i] < s[j]
}

func (s sortablePorts) Swap(i, j int) {
	t := s[i]
	s[i] = s[j]
	s[j] = t
}

type OutputWriter interface {
	Print(values ...interface{})
	PrintNoTab(values ...interface{})
	PrintColorNoTab(ctx *ansiterm.Context, value interface{})
}

func printPorts(w OutputWriter, ps []string) {
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
				w.PrintNoTab(" ")
			}
			if hasPrev {
				w.PrintNoTab(",")
			}
			hasPrev = true
			split := protocol.components[port]
			group := protocol.groups[port]
			// Print grouped ports.
			if len(group) > 0 {
				last := group[len(group)-1]
				lastSplit := protocol.components[last]
				portRange := fmt.Sprintf("%s-%s", split[0], lastSplit[0])
				w.PrintColorNoTab(output.EmphasisHighlight.BrightMagenta, portRange)
				continue
			}
			// Print single port with protocol.
			if len(split) > 1 {
				w.PrintColorNoTab(output.EmphasisHighlight.BrightMagenta, split[0])
				continue
			}
			// Everything else.
			break
		}
		if hasPrev {
			hasOutput = true
			if _, err := strconv.Atoi(pk); err == nil {
				w.PrintColorNoTab(output.EmphasisHighlight.BrightMagenta, pk)
			} else {
				w.PrintNoTab(pk)
			}
		}
	}

	w.Print("") //Print empty tab after the ports
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
			logger.Errorf(context.TODO(), "invalid offer URL %q: %v", app.OfferURL, err)
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

	w := startSection(tw, false, "Integration provider", "Requirer", "Interface", "Type", "Message")

	for _, r := range relations {
		provider := strings.Split(r.Provider, ":")
		w.PrintNoTab(provider[0]) //the service name (mysql)
		if len(provider) > 1 {
			w.PrintColor(output.EmphasisHighlight.Magenta, fmt.Sprintf(":%s", provider[1])) //the resource type (:cluster)
		}
		requirer := strings.Split(r.Requirer, ":")
		w.PrintNoTab(requirer[0]) //the service name (mysql)
		if len(requirer) > 1 {
			w.PrintColor(output.EmphasisHighlight.Magenta, fmt.Sprintf(":%s", requirer[1])) //the resource type (:cluster)
		}
		w.Print(r.Interface, r.Type)
		if r.Status != string(relation.Joined) {
			w.PrintColor(cmdcrossmodel.RelationStatusColor(relation.Status(r.Status)), r.Status)
			w.PrintColorNoTab(output.EmphasisHighlight.Gray, truncateMessage(r.Message))
		}
		w.Println()
	}
	endSection(tw)
}

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
			w.Println("", "", "", "", "", endpointName, endpoint.Interface, endpoint.Role)
		}
	}
	endSection(tw)
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
	w := startSection(tw, standAlone, "Machine", "State", "Address", "Inst id", "Base", "AZ", "Message")
	for _, name := range naturalsort.Sort(stringKeysFromMap(machines)) {
		printMachine(w, machines[name])
	}
	endSection(tw)
}

func printMachine(w *output.Wrapper, m machineStatus) {
	// We want to display availability zone so extract from hardware info".
	hw, err := instance.ParseHardware(m.Hardware)
	if err != nil {
		logger.Warningf(context.TODO(), "invalid hardware info %s for machine %v", m.Hardware, m)
	}
	az := ""
	if hw.AvailabilityZone != nil {
		az = *hw.AvailabilityZone
	}

	status, message := getStatusAndMessageFromMachineStatus(m)

	w.Print(m.Id)
	w.PrintStatus(status)
	w.PrintColor(output.InfoHighlight, m.DNSName)
	baseStr := ""
	if m.Base != nil {
		base, err := corebase.ParseBase(m.Base.Name, m.Base.Channel)
		if err == nil {
			baseStr = base.DisplayString()
		}
	}
	w.Print(m.machineName(), baseStr, az)
	if message != "" { //some unit tests were failing because of the printed empty string .
		w.PrintColorNoTab(output.EmphasisHighlight.Gray, truncateMessage(message))
	}
	w.Println()

	for _, name := range naturalsort.Sort(stringKeysFromMap(m.Containers)) {
		printMachine(w, m.Containers[name])
	}
}

// Apply some rules around what we show in the tabular format
// Rules:
//   - if the modification-status is in error mode, then show that over the
//     juju status and machine status message
func getStatusAndMessageFromMachineStatus(m machineStatus) (status.Status, string) {
	currentStatus := m.JujuStatus.Current
	currentMessage := m.MachineStatus.Message
	if m.ModificationStatus.Current == status.Error {
		currentStatus = m.ModificationStatus.Current
		currentMessage = m.ModificationStatus.Message
	}

	return currentStatus, currentMessage
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

// truncateMessage truncates the given message if it is too long.
func truncateMessage(msg string) string {
	if len(msg) > maxMessageLength {
		return msg[:maxMessageLength-len(ellipsis)] + ellipsis
	}
	return msg
}
