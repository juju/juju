// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/juju/core/lxdprofile"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
)

func agentStatusFromStatusInfo(s []status.StatusInfo, kind status.HistoryKind) []params.DetailedStatus {
	result := []params.DetailedStatus{}
	for _, v := range s {
		result = append(result, params.DetailedStatus{
			Status: string(v.Status),
			Info:   v.Message,
			Data:   v.Data,
			Since:  v.Since,
			Kind:   string(kind),
		})
	}
	return result

}

type byTime []params.DetailedStatus

func (s byTime) Len() int {
	return len(s)
}
func (s byTime) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byTime) Less(i, j int) bool {
	return s[i].Since.Before(*s[j].Since)
}

// unitStatusHistory returns a list of status history entries for unit agents or workloads.
func (c *Client) unitStatusHistory(unitTag names.UnitTag, filter status.StatusHistoryFilter, kind status.HistoryKind) ([]params.DetailedStatus, error) {
	unit, err := c.api.stateAccessor.Unit(unitTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	statuses := []params.DetailedStatus{}
	if kind == status.KindUnit || kind == status.KindWorkload {
		unitStatuses, err := unit.StatusHistory(filter)
		if err != nil {
			return nil, errors.Trace(err)
		}
		statuses = agentStatusFromStatusInfo(unitStatuses, status.KindWorkload)

	}
	if kind == status.KindUnit || kind == status.KindUnitAgent {
		agentStatuses, err := unit.AgentHistory().StatusHistory(filter)
		if err != nil {
			return nil, errors.Trace(err)
		}
		statuses = append(statuses, agentStatusFromStatusInfo(agentStatuses, status.KindUnitAgent)...)
	}

	sort.Sort(byTime(statuses))
	if kind == status.KindUnit && filter.Size > 0 {
		if len(statuses) > filter.Size {
			statuses = statuses[len(statuses)-filter.Size:]
		}
	}

	return statuses, nil
}

// machineStatusHistory returns status history for the given machine.
func (c *Client) machineStatusHistory(machineTag names.MachineTag, filter status.StatusHistoryFilter, kind status.HistoryKind) ([]params.DetailedStatus, error) {
	machine, err := c.api.stateAccessor.Machine(machineTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	var sInfo []status.StatusInfo
	if kind == status.KindMachineInstance || kind == status.KindContainerInstance {
		sInfo, err = machine.InstanceStatusHistory(filter)
	} else {
		sInfo, err = machine.StatusHistory(filter)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return agentStatusFromStatusInfo(sInfo, kind), nil
}

// StatusHistory returns a slice of past statuses for several entities.
func (c *Client) StatusHistory(request params.StatusHistoryRequests) params.StatusHistoryResults {

	results := params.StatusHistoryResults{}
	// TODO(perrito666) the contents of the loop could be split into
	// a oneHistory method for clarity.
	for _, request := range request.Requests {
		filter := status.StatusHistoryFilter{
			Size:     request.Filter.Size,
			FromDate: request.Filter.Date,
			Delta:    request.Filter.Delta,
			Exclude:  set.NewStrings(request.Filter.Exclude...),
		}
		if err := c.checkCanRead(); err != nil {
			history := params.StatusHistoryResult{
				Error: common.ServerError(err),
			}
			results.Results = append(results.Results, history)
			continue

		}

		if err := filter.Validate(); err != nil {
			history := params.StatusHistoryResult{
				Error: common.ServerError(errors.Annotate(err, "cannot validate status history filter")),
			}
			results.Results = append(results.Results, history)
			continue
		}

		var (
			err  error
			hist []params.DetailedStatus
		)
		kind := status.HistoryKind(request.Kind)
		err = errors.NotValidf("%q requires a unit, got %T", kind, request.Tag)
		switch kind {
		case status.KindUnit, status.KindWorkload, status.KindUnitAgent:
			var u names.UnitTag
			if u, err = names.ParseUnitTag(request.Tag); err == nil {
				hist, err = c.unitStatusHistory(u, filter, kind)
			}
		default:
			var m names.MachineTag
			if m, err = names.ParseMachineTag(request.Tag); err == nil {
				hist, err = c.machineStatusHistory(m, filter, kind)
			}
		}

		if err == nil {
			sort.Sort(byTime(hist))
		}

		results.Results = append(results.Results,
			params.StatusHistoryResult{
				History: params.History{Statuses: hist},
				Error:   common.ServerError(errors.Annotatef(err, "fetching status history for %q", request.Tag)),
			})
	}
	return results
}

// FullStatus gives the information needed for juju status over the api
func (c *Client) FullStatus(args params.StatusParams) (params.FullStatus, error) {
	if err := c.checkCanRead(); err != nil {
		return params.FullStatus{}, err
	}

	var noStatus params.FullStatus
	var context statusContext

	m, err := c.api.stateAccessor.Model()
	if err != nil {
		return noStatus, errors.Annotate(err, "cannot get model")
	}
	context.presence.Presence = c.api.presence.ModelPresence(m.UUID())
	cfg, err := m.Config()
	if err != nil {
		return noStatus, errors.Annotate(err, "cannot obtain current model config")
	}
	context.providerType = cfg.Type()

	if context.model, err = c.api.stateAccessor.Model(); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch model")
	}
	if context.status, err = context.model.LoadModelStatus(); err != nil {
		return noStatus, errors.Annotate(err, "could not load model status values")
	}
	if context.allAppsUnitsCharmBindings, err =
		fetchAllApplicationsAndUnits(c.api.stateAccessor, context.model); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch applications and units")
	}
	if context.consumerRemoteApplications, err =
		fetchConsumerRemoteApplications(c.api.stateAccessor); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch remote applications")
	}
	// Only admins can see offer details.
	if err := c.checkIsAdmin(); err == nil {
		if context.offers, err =
			fetchOffers(c.api.stateAccessor, context.allAppsUnitsCharmBindings.applications); err != nil {
			return noStatus, errors.Annotate(err, "could not fetch application offers")
		}
	}
	if context.machines, err = fetchMachines(c.api.stateAccessor, nil); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch machines")
	}
	// These may be empty when machines have not finished deployment.
	if context.ipAddresses, context.spaces, context.linkLayerDevices, err =
		fetchNetworkInterfaces(c.api.stateAccessor); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch IP addresses and link layer devices")
	}
	if context.relations, context.relationsById, err = fetchRelations(c.api.stateAccessor); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch relations")
	}
	if len(context.allAppsUnitsCharmBindings.applications) > 0 {
		if context.leaders, err = c.api.stateAccessor.ApplicationLeaders(); err != nil {
			return noStatus, errors.Annotate(err, "could not fetch leaders")
		}
	}
	if context.controllerTimestamp, err = c.api.stateAccessor.ControllerTimestamp(); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch controller timestamp")
	}

	logger.Tracef("Applications: %v", context.allAppsUnitsCharmBindings.applications)
	logger.Tracef("Remote applications: %v", context.consumerRemoteApplications)
	logger.Tracef("Offers: %v", context.offers)
	logger.Tracef("Relations: %v", context.relations)

	if len(args.Patterns) > 0 {
		predicate := BuildPredicateFor(args.Patterns)

		// First, attempt to match machines. Any units on those
		// machines are implicitly matched.
		matchedMachines := make(set.Strings)
		for _, machineList := range context.machines {
			for _, m := range machineList {
				matches, err := predicate(m)
				if err != nil {
					return noStatus, errors.Annotate(
						err, "could not filter machines",
					)
				}
				if matches {
					matchedMachines.Add(m.Id())
				}
			}
		}

		// Filter units
		matchedSvcs := make(set.Strings)
		unitChainPredicate := UnitChainPredicateFn(predicate, context.unitByName)
		// It's possible that we will discover a unit that matches given filter
		// half way through units collection. In that case, it may be that the machine
		// for that unit has other applications' units on it that have already been examined
		// prior. This means that we may miss these other application(s).
		// This behavior has been inconsistent since we get units in a map where
		// the order is not guaranteed.
		// To cater for this scenario, we need to gather all units
		// in a temporary collection keyed on machine to allow for the later
		// pass. This fixes situations similar to inconsistencies
		// observed in lp#1592872.
		machineUnits := map[string][]string{}
		for _, unitMap := range context.allAppsUnitsCharmBindings.units {
			for name, unit := range unitMap {
				machineId, err := unit.AssignedMachineId()
				if err != nil {
					machineId = ""
				} else if matchedMachines.Contains(machineId) {
					// Unit is on a matching machine.
					matchedSvcs.Add(unit.ApplicationName())
					continue
				}
				if machineId != "" {
					machineUnits[machineId] = append(machineUnits[machineId], unit.ApplicationName())
				}

				// Always start examining at the top-level. This
				// prevents a situation where we filter a subordinate
				// before we discover its parent is a match.
				if !unit.IsPrincipal() {
					continue
				} else if matches, err := unitChainPredicate(unit); err != nil {
					return noStatus, errors.Annotate(err, "could not filter units")
				} else if !matches {
					delete(unitMap, name)
					continue
				}
				matchedSvcs.Add(unit.ApplicationName())
				if machineId != "" {
					matchedMachines.Add(machineId)
				}
			}
		}
		for _, m := range matchedMachines.SortedValues() {
			for _, a := range machineUnits[m] {
				if !matchedSvcs.Contains(a) {
					matchedSvcs.Add(a)
				}
			}
		}

		// Filter applications
		for appName, app := range context.allAppsUnitsCharmBindings.applications {
			matches, err := predicate(app)
			if err != nil {
				return noStatus, errors.Annotate(err, "could not filter applications")
			}

			// There are matched units for this application
			// or the application matched the given criteria.
			deleted := false
			if !matchedSvcs.Contains(appName) && !matches {
				delete(context.allAppsUnitsCharmBindings.applications, appName)
				deleted = true
			}

			// Filter relations:
			// Remove relations for applications that were deleted and
			// for the applications that did not match the
			// given criteria.
			if deleted || !matches {
				// delete relations for this app
				if relations, ok := context.relations[appName]; ok {
					for _, r := range relations {
						delete(context.relationsById, r.Id())
					}
					delete(context.relations, appName)
				}
			}
		}
		// TODO(wallyworld) - filter remote applications

		// Filter machines
		for status, machineList := range context.machines {
			matched := make([]*state.Machine, 0, len(machineList))
			for _, m := range machineList {
				machineContainers, err := m.Containers()
				if err != nil {
					return noStatus, err
				}
				machineContainersSet := set.NewStrings(machineContainers...)

				if matchedMachines.Contains(m.Id()) || !matchedMachines.Intersection(machineContainersSet).IsEmpty() {
					// The machine is matched directly, or contains a unit
					// or container that matches.
					logger.Tracef("machine %s is hosting something.", m.Id())
					matched = append(matched, m)
					continue
				}
			}
			context.machines[status] = matched
		}
	}

	modelStatus, err := c.modelStatus()
	if err != nil {
		return noStatus, errors.Annotate(err, "cannot determine model status")
	}
	return params.FullStatus{
		Model:               modelStatus,
		Machines:            context.processMachines(),
		Applications:        context.processApplications(),
		RemoteApplications:  context.processRemoteApplications(),
		Offers:              context.processOffers(),
		Relations:           context.processRelations(),
		ControllerTimestamp: context.controllerTimestamp,
	}, nil
}

// newToolsVersionAvailable will return a string representing a tools
// version only if the latest check is newer than current tools.
func (c *Client) modelStatus() (params.ModelStatusInfo, error) {
	var info params.ModelStatusInfo

	m, err := c.api.stateAccessor.Model()
	if err != nil {
		return info, errors.Annotate(err, "cannot get model")
	}
	info.Name = m.Name()
	info.Type = string(m.Type())
	info.CloudTag = names.NewCloudTag(m.Cloud()).String()
	info.CloudRegion = m.CloudRegion()

	cfg, err := m.Config()
	if err != nil {
		return params.ModelStatusInfo{}, errors.Annotate(err, "cannot obtain current model config")
	}

	latestVersion := m.LatestToolsVersion()
	current, ok := cfg.AgentVersion()
	if ok {
		info.Version = current.String()
		if current.Compare(latestVersion) < 0 {
			info.AvailableVersion = latestVersion.String()
		}
	}

	status, err := m.Status()
	if err != nil {
		return params.ModelStatusInfo{}, errors.Annotate(err, "cannot obtain model status info")
	}

	info.SLA = m.SLALevel()

	info.ModelStatus = params.DetailedStatus{
		Status: status.Status.String(),
		Info:   status.Message,
		Since:  status.Since,
		Data:   status.Data,
	}

	if info.SLA != "unsupported" {
		ms := m.MeterStatus()
		if isColorStatus(ms.Code) {
			info.MeterStatus = params.MeterStatus{Color: strings.ToLower(ms.Code.String()), Message: ms.Info}
		}
	}

	return info, nil
}

type applicationStatusInfo struct {
	// application: application name -> application
	applications map[string]*state.Application

	// units: units name -> units
	units map[string]map[string]*state.Unit

	// latestcharm: charm URL -> charm
	latestCharms map[charm.URL]*state.Charm

	// endpointpointBindings: application name -> endpoint -> space
	endpointBindings map[string]map[string]string

	// lxdProfiles: lxd profile name -> lxd profile
	lxdProfiles map[string]*charm.LXDProfile
}

type statusContext struct {
	providerType string
	model        *state.Model
	status       *state.ModelStatus
	presence     common.ModelPresenceContext

	// machines: top-level machine id -> list of machines nested in
	// this machine.
	machines map[string][]*state.Machine

	// ipAddresses: machine id -> list of ip.addresses
	ipAddresses map[string][]*state.Address

	// spaces: machine id -> deviceName -> list of spaceNames
	spaces map[string]map[string]set.Strings

	// linkLayerDevices: machine id -> list of linkLayerDevices
	linkLayerDevices map[string][]*state.LinkLayerDevice

	// remote applications: application name -> application
	consumerRemoteApplications map[string]*state.RemoteApplication

	// offers: offer name -> offer
	offers map[string]offerStatus

	// controller current timestamp
	controllerTimestamp *time.Time

	allAppsUnitsCharmBindings applicationStatusInfo
	relations                 map[string][]*state.Relation
	relationsById             map[int]*state.Relation
	units                     map[string]map[string]*state.Unit
	latestCharms              map[charm.URL]*state.Charm
	leaders                   map[string]string
}

// fetchMachines returns a map from top level machine id to machines, where machines[0] is the host
// machine and machines[1..n] are any containers (including nested ones).
//
// If machineIds is non-nil, only machines whose IDs are in the set are returned.
func fetchMachines(st Backend, machineIds set.Strings) (map[string][]*state.Machine, error) {
	v := make(map[string][]*state.Machine)
	machines, err := st.AllMachines()
	if err != nil {
		return nil, err
	}
	// AllMachines gives us machines sorted by id.
	for _, m := range machines {
		if machineIds != nil && !machineIds.Contains(m.Id()) {
			continue
		}
		parentId, ok := m.ParentId()
		if !ok {
			// Only top level host machines go directly into the machine map.
			v[m.Id()] = []*state.Machine{m}
		} else {
			topParentId := state.TopParentId(m.Id())
			machines, ok := v[topParentId]
			if !ok {
				panic(fmt.Errorf("unexpected machine id %q", parentId))
			}
			machines = append(machines, m)
			v[topParentId] = machines
		}
	}
	return v, nil
}

// fetchNetworkInterfaces returns maps from machine id to ip.addresses, machine
// id to a map of interface names from space names, and machine id to
// linklayerdevices.
//
// All are required to determine a machine's network interfaces configuration,
// so we want all or none.
func fetchNetworkInterfaces(st Backend) (map[string][]*state.Address, map[string]map[string]set.Strings, map[string][]*state.LinkLayerDevice, error) {
	ipAddresses := make(map[string][]*state.Address)
	spaces := make(map[string]map[string]set.Strings)
	subnets, err := st.AllSubnets()
	if err != nil {
		return nil, nil, nil, err
	}
	subnetsByCIDR := make(map[string]*state.Subnet)
	for _, subnet := range subnets {
		subnetsByCIDR[subnet.CIDR()] = subnet
	}
	// For every machine, track what devices have addresses so we can filter linklayerdevices later
	devicesWithAddresses := make(map[string]set.Strings)
	ipAddrs, err := st.AllIPAddresses()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, ipAddr := range ipAddrs {
		if ipAddr.LoopbackConfigMethod() {
			continue
		}
		machineID := ipAddr.MachineID()
		ipAddresses[machineID] = append(ipAddresses[machineID], ipAddr)
		if subnet, ok := subnetsByCIDR[ipAddr.SubnetCIDR()]; ok {
			if spaceName := subnet.SpaceName(); spaceName != "" {
				devices, ok := spaces[machineID]
				if !ok {
					devices = make(map[string]set.Strings)
					spaces[machineID] = devices
				}
				deviceName := ipAddr.DeviceName()
				spacesSet, ok := devices[deviceName]
				if !ok {
					spacesSet = make(set.Strings)
					devices[deviceName] = spacesSet
				}
				spacesSet.Add(spaceName)
			}
		}
		deviceSet, ok := devicesWithAddresses[machineID]
		if ok {
			deviceSet.Add(ipAddr.DeviceName())
		} else {
			devicesWithAddresses[machineID] = set.NewStrings(ipAddr.DeviceName())
		}
	}

	linkLayerDevices := make(map[string][]*state.LinkLayerDevice)
	llDevs, err := st.AllLinkLayerDevices()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, llDev := range llDevs {
		if llDev.IsLoopbackDevice() {
			continue
		}
		machineID := llDev.MachineID()
		machineDevs, ok := devicesWithAddresses[machineID]
		if !ok {
			// This machine ID doesn't seem to have any devices with IP Addresses
			continue
		}
		if !machineDevs.Contains(llDev.Name()) {
			// this device did not have any IP Addresses
			continue
		}
		// This device had an IP Address, so include it in the list of devices for this machine
		linkLayerDevices[machineID] = append(linkLayerDevices[machineID], llDev)
	}

	return ipAddresses, spaces, linkLayerDevices, nil
}

// fetchAllApplicationsAndUnits returns a map from application name to application,
// a map from application name to unit name to unit, and a map from base charm URL to latest URL.
func fetchAllApplicationsAndUnits(
	st Backend,
	model *state.Model,
) (applicationStatusInfo, error) {
	appMap := make(map[string]*state.Application)
	unitMap := make(map[string]map[string]*state.Unit)
	latestCharms := make(map[charm.URL]*state.Charm)
	applications, err := st.AllApplications()
	units, err := model.AllUnits()
	if err != nil {
		return applicationStatusInfo{}, err
	}
	allUnitsByApp := make(map[string]map[string]*state.Unit)
	for _, unit := range units {
		appName := unit.ApplicationName()

		if inner, found := allUnitsByApp[appName]; found {
			inner[unit.Name()] = unit
		} else {
			allUnitsByApp[appName] = map[string]*state.Unit{
				unit.Name(): unit,
			}
		}
	}

	endpointBindings, err := model.AllEndpointBindings()
	if err != nil {
		return applicationStatusInfo{}, err
	}
	allBindingsByApp := make(map[string]map[string]string)
	for _, bindings := range endpointBindings {
		allBindingsByApp[bindings.AppName] = bindings.Bindings
	}

	lxdProfiles := make(map[string]*charm.LXDProfile)
	for _, app := range applications {
		appMap[app.Name()] = app
		appUnits := allUnitsByApp[app.Name()]
		charmURL, _ := app.CharmURL()

		if len(appUnits) > 0 {
			unitMap[app.Name()] = appUnits
			// Record the base URL for the application's charm so that
			// the latest store revision can be looked up.
			if charmURL.Schema == "cs" {
				latestCharms[*charmURL.WithRevision(-1)] = nil
			}
		}

		ch, _, err := app.Charm()
		if err != nil {
			continue
		}
		chName := lxdprofile.Name(model.Name(), app.Name(), ch.Revision())
		lxdProfiles[chName] = ch.LXDProfile()
	}

	for baseURL := range latestCharms {
		ch, err := st.LatestPlaceholderCharm(&baseURL)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return applicationStatusInfo{}, err
		}
		latestCharms[baseURL] = ch
	}

	return applicationStatusInfo{
		applications:     appMap,
		units:            unitMap,
		latestCharms:     latestCharms,
		endpointBindings: allBindingsByApp,
		lxdProfiles:      lxdProfiles,
	}, nil
}

// fetchConsumerRemoteApplications returns a map from application name to remote application.
func fetchConsumerRemoteApplications(st Backend) (map[string]*state.RemoteApplication, error) {
	appMap := make(map[string]*state.RemoteApplication)
	applications, err := st.AllRemoteApplications()
	if err != nil {
		return nil, err
	}
	for _, a := range applications {
		if _, ok := a.URL(); !ok {
			continue
		}
		appMap[a.Name()] = a
	}
	return appMap, nil
}

// fetchOfferConnections returns a map from relation id to offer connection.
func fetchOffers(st Backend, applications map[string]*state.Application) (map[string]offerStatus, error) {
	offersMap := make(map[string]offerStatus)
	offers, err := st.AllApplicationOffers()
	if err != nil {
		return nil, err
	}
	for _, offer := range offers {
		offerInfo := offerStatus{
			ApplicationOffer: crossmodel.ApplicationOffer{
				OfferName:       offer.OfferName,
				OfferUUID:       offer.OfferUUID,
				ApplicationName: offer.ApplicationName,
				Endpoints:       offer.Endpoints,
			},
		}
		app, ok := applications[offer.ApplicationName]
		if !ok {
			continue
		}
		curl, _ := app.CharmURL()
		offerInfo.charmURL = curl.String()
		rc, err := st.RemoteConnectionStatus(offer.OfferUUID)
		if err != nil && !errors.IsNotFound(err) {
			offerInfo.err = err
			continue
		} else if err == nil {
			offerInfo.totalConnectedCount = rc.TotalConnectionCount()
			offerInfo.activeConnectedCount = rc.ActiveConnectionCount()
		}
		offersMap[offer.OfferName] = offerInfo
	}
	return offersMap, nil
}

// fetchRelations returns a map of all relations keyed by application name,
// and another map keyed by id..
//
// This structure is useful for processApplicationRelations() which needs
// to have the relations for each application. Reading them once here
// avoids the repeated DB hits to retrieve the relations for each
// application that used to happen in processApplicationRelations().
func fetchRelations(st Backend) (map[string][]*state.Relation, map[int]*state.Relation, error) {
	relations, err := st.AllRelations()
	if err != nil {
		return nil, nil, err
	}
	out := make(map[string][]*state.Relation)
	outById := make(map[int]*state.Relation)
	for _, relation := range relations {
		outById[relation.Id()] = relation
		// If either end of the relation is a remote application
		// on the offering side, exclude it here.
		isRemote := false
		for _, ep := range relation.Endpoints() {
			if app, err := st.RemoteApplication(ep.ApplicationName); err == nil {
				if app.IsConsumerProxy() {
					isRemote = true
					break
				}
			} else if !errors.IsNotFound(err) {
				return nil, nil, err
			}
		}
		if isRemote {
			continue
		}
		for _, ep := range relation.Endpoints() {
			out[ep.ApplicationName] = append(out[ep.ApplicationName], relation)
		}
	}
	return out, outById, nil
}

func (c *statusContext) processMachines() map[string]params.MachineStatus {
	machinesMap := make(map[string]params.MachineStatus)
	cache := make(map[string]params.MachineStatus)
	for id, machines := range c.machines {

		if len(machines) <= 0 {
			continue
		}

		// Element 0 is assumed to be the top-level machine.
		tlMachine := machines[0]
		hostStatus := c.makeMachineStatus(tlMachine, c.allAppsUnitsCharmBindings)
		machinesMap[id] = hostStatus
		cache[id] = hostStatus

		for _, machine := range machines[1:] {
			parent, ok := cache[state.ParentId(machine.Id())]
			if !ok {
				logger.Errorf("programmer error, please file a bug, reference this whole log line: %q, %q", id, machine.Id())
				continue
			}

			status := c.makeMachineStatus(machine, c.allAppsUnitsCharmBindings)
			parent.Containers[machine.Id()] = status
			cache[machine.Id()] = status
		}
	}
	return machinesMap
}

func (c *statusContext) makeMachineStatus(machine *state.Machine, appStatusInfo applicationStatusInfo) (status params.MachineStatus) {
	machineID := machine.Id()
	ipAddresses := c.ipAddresses[machineID]
	spaces := c.spaces[machineID]
	linkLayerDevices := c.linkLayerDevices[machineID]

	var err error
	status.Id = machine.Id()
	agentStatus := c.processMachine(machine)
	status.AgentStatus = agentStatus

	status.Series = machine.Series()
	status.Jobs = paramsJobsFromJobs(machine.Jobs())
	status.WantsVote = machine.WantsVote()
	status.HasVote = machine.HasVote()
	sInfo, err := c.status.MachineInstance(machineID)
	populateStatusFromStatusInfoAndErr(&status.InstanceStatus, sInfo, err)
	// TODO: fetch all instance data for machines in one go.
	instid, err := machine.InstanceId()
	if err == nil {
		status.InstanceId = instid
		addr, err := machine.PublicAddress()
		if err != nil {
			// Usually this indicates that no addresses have been set on the
			// machine yet.
			addr = network.Address{}
			logger.Debugf("error fetching public address: %q", err)
		}
		status.DNSName = addr.Value
		mAddrs := machine.Addresses()
		if len(mAddrs) == 0 {
			logger.Debugf("no IP addresses fetched for machine %q", instid)
			// At least give it the newly created DNSName address, if it exists.
			if addr.Value != "" {
				mAddrs = append(mAddrs, addr)
			}
		}
		for _, mAddr := range mAddrs {
			switch mAddr.Scope {
			case network.ScopeMachineLocal, network.ScopeLinkLocal:
				continue
			}
			status.IPAddresses = append(status.IPAddresses, mAddr.Value)
		}
		status.NetworkInterfaces = make(map[string]params.NetworkInterface, len(linkLayerDevices))
		for _, llDev := range linkLayerDevices {
			device := llDev.Name()
			ips := []string{}
			gw := []string{}
			ns := []string{}
			sp := make(set.Strings)
			for _, ipAddress := range ipAddresses {
				if ipAddress.DeviceName() != device {
					continue
				}
				ips = append(ips, ipAddress.Value())
				// We don't expect to find more than one
				// ipAddress on a device with a list of
				// nameservers, but append in any case.
				if len(ipAddress.DNSServers()) > 0 {
					ns = append(ns, ipAddress.DNSServers()...)
				}
				// There should only be one gateway per device
				// (per machine, in fact, as we don't store
				// metrics). If we find more than one we should
				// show them all.
				if ipAddress.GatewayAddress() != "" {
					gw = append(gw, ipAddress.GatewayAddress())
				}
				// There should only be one space per address,
				// but it's technically possible to have more
				// than one address on an interface. If we find
				// that happens, we need to show all spaces, to
				// be safe.
				sp = spaces[device]
			}
			status.NetworkInterfaces[device] = params.NetworkInterface{
				IPAddresses:    ips,
				MACAddress:     llDev.MACAddress(),
				Gateway:        strings.Join(gw, " "),
				DNSNameservers: ns,
				Space:          strings.Join(sp.Values(), " "),
				IsUp:           llDev.IsUp(),
			}
		}
		logger.Tracef("NetworkInterfaces: %+v", status.NetworkInterfaces)
	} else {
		if errors.IsNotProvisioned(err) {
			status.InstanceId = "pending"
		} else {
			status.InstanceId = "error"
		}
	}
	// TODO: preload all constraints.
	constraints, err := machine.Constraints()
	if err != nil {
		if !errors.IsNotFound(err) {
			status.Constraints = "error"
		}
	} else {
		status.Constraints = constraints.String()
	}
	// TODO: preload all hardware characteristics.
	hc, err := machine.HardwareCharacteristics()
	if err != nil {
		if !errors.IsNotFound(err) {
			status.Hardware = "error"
		}
	} else {
		status.Hardware = hc.String()
	}
	status.Containers = make(map[string]params.MachineStatus)

	lxdProfiles := make(map[string]params.LXDProfile)
	charmProfiles, err := machine.CharmProfiles()
	if err == nil {
		for _, v := range charmProfiles {
			if profile, ok := appStatusInfo.lxdProfiles[v]; ok {
				lxdProfiles[v] = params.LXDProfile{
					Config:      profile.Config,
					Description: profile.Description,
					Devices:     profile.Devices,
				}
			}
		}
	} else {
		logger.Debugf("error fetching lxd profiles for %s: %q", machine.String(), err.Error())
	}
	status.LXDProfiles = lxdProfiles

	return
}

func (context *statusContext) processRelations() []params.RelationStatus {
	var out []params.RelationStatus
	relations := context.getAllRelations()
	for _, relation := range relations {
		var eps []params.EndpointStatus
		var scope charm.RelationScope
		var relationInterface string
		for _, ep := range relation.Endpoints() {
			eps = append(eps, params.EndpointStatus{
				ApplicationName: ep.ApplicationName,
				Name:            ep.Name,
				Role:            string(ep.Role),
				Subordinate:     context.isSubordinate(&ep),
			})
			// these should match on both sides so use the last
			relationInterface = ep.Interface
			scope = ep.Scope
		}
		relStatus := params.RelationStatus{
			Id:        relation.Id(),
			Key:       relation.String(),
			Interface: relationInterface,
			Scope:     string(scope),
			Endpoints: eps,
		}
		rStatus, err := relation.Status()
		populateStatusFromStatusInfoAndErr(&relStatus.Status, rStatus, err)
		out = append(out, relStatus)
	}
	return out
}

// This method exists only to dedup the loaded relations as they will
// appear multiple times in context.relations.
func (context *statusContext) getAllRelations() []*state.Relation {
	var out []*state.Relation
	seenRelations := make(map[int]bool)
	for _, relations := range context.relations {
		for _, relation := range relations {
			if _, found := seenRelations[relation.Id()]; !found {
				out = append(out, relation)
				seenRelations[relation.Id()] = true
			}
		}
	}
	return out
}

func (context *statusContext) isSubordinate(ep *state.Endpoint) bool {
	application := context.allAppsUnitsCharmBindings.applications[ep.ApplicationName]
	if application == nil {
		return false
	}
	return isSubordinate(ep, application)
}

func isSubordinate(ep *state.Endpoint, application *state.Application) bool {
	return ep.Scope == charm.ScopeContainer && !application.IsPrincipal()
}

// paramsJobsFromJobs converts state jobs to params jobs.
func paramsJobsFromJobs(jobs []state.MachineJob) []multiwatcher.MachineJob {
	paramsJobs := make([]multiwatcher.MachineJob, len(jobs))
	for i, machineJob := range jobs {
		paramsJobs[i] = machineJob.ToParams()
	}
	return paramsJobs
}

func (context *statusContext) processApplications() map[string]params.ApplicationStatus {
	applicationsMap := make(map[string]params.ApplicationStatus)
	for _, s := range context.allAppsUnitsCharmBindings.applications {
		applicationsMap[s.Name()] = context.processApplication(s)
	}
	return applicationsMap
}

func (context *statusContext) processApplication(application *state.Application) params.ApplicationStatus {
	applicationCharm, _, err := application.Charm()
	if err != nil {
		return params.ApplicationStatus{Err: common.ServerError(err)}
	}

	var processedStatus = params.ApplicationStatus{
		Charm:        applicationCharm.URL().String(),
		Series:       application.Series(),
		Exposed:      application.IsExposed(),
		Life:         processLife(application),
		CharmVersion: applicationCharm.Version(),
	}

	if latestCharm, ok := context.allAppsUnitsCharmBindings.latestCharms[*applicationCharm.URL().WithRevision(-1)]; ok && latestCharm != nil {
		if latestCharm.Revision() > applicationCharm.URL().Revision {
			processedStatus.CanUpgradeTo = latestCharm.String()
		}
	}

	processedStatus.Relations, processedStatus.SubordinateTo, err = context.processApplicationRelations(application)
	if err != nil {
		processedStatus.Err = common.ServerError(err)
		return processedStatus
	}
	units := context.allAppsUnitsCharmBindings.units[application.Name()]
	if application.IsPrincipal() {
		processedStatus.Units = context.processUnits(units, applicationCharm.URL().String())
	}
	var unitNames []string
	for _, unit := range units {
		unitNames = append(unitNames, unit.Name())
	}
	applicationStatus, err := context.status.Application(application.Name(), unitNames)
	if err != nil {
		processedStatus.Err = common.ServerError(err)
		return processedStatus
	}
	processedStatus.Status.Status = applicationStatus.Status.String()
	processedStatus.Status.Info = applicationStatus.Message
	processedStatus.Status.Data = applicationStatus.Data
	processedStatus.Status.Since = applicationStatus.Since

	metrics := applicationCharm.Metrics()
	planRequired := metrics != nil && metrics.Plan != nil && metrics.Plan.Required
	if planRequired || len(application.MetricCredentials()) > 0 {
		processedStatus.MeterStatuses = context.processUnitMeterStatuses(units)
	}

	// TODO(caas) - there's no way for a CAAS charm to set workload version yet
	if context.model.Type() == state.ModelTypeIAAS {
		versions := make([]status.StatusInfo, 0, len(units))
		for _, unit := range units {
			workloadVersion, err := context.status.FullUnitWorkloadVersion(unit.Name())
			if err != nil {
				processedStatus.Err = common.ServerError(err)
				return processedStatus
			}
			versions = append(versions, workloadVersion)
		}
		if len(versions) > 0 {
			sort.Sort(bySinceDescending(versions))
			processedStatus.WorkloadVersion = versions[0].Message
		}
	} else {
		// We'll punt on using the docker image name.
		caasModel, err := context.model.CAASModel()
		if err != nil {
			return params.ApplicationStatus{Err: common.ServerError(err)}
		}
		specStr, err := caasModel.PodSpec(application.ApplicationTag())
		if err != nil && !errors.IsNotFound(err) {
			return params.ApplicationStatus{Err: common.ServerError(err)}
		}
		if specStr != "" {
			provider, err := environs.Provider(context.providerType)
			if err != nil {
				return params.ApplicationStatus{Err: common.ServerError(err)}
			}
			caasProvider, ok := provider.(caas.ContainerEnvironProvider)
			if !ok {
				err := errors.NotValidf("container environ provider %T", provider)
				return params.ApplicationStatus{Err: common.ServerError(err)}
			}
			spec, err := caasProvider.ParsePodSpec(specStr)
			if err != nil {
				return params.ApplicationStatus{Err: common.ServerError(err)}
			}
			// Container zero is the primary.
			processedStatus.WorkloadVersion = fmt.Sprintf("%v", spec.Containers[0].Image)
		}
		serviceInfo, err := application.ServiceInfo()
		if err == nil {
			processedStatus.ProviderId = serviceInfo.ProviderId()
			if len(serviceInfo.Addresses()) > 0 {
				processedStatus.PublicAddress = serviceInfo.Addresses()[0].Value
			}
		} else {
			logger.Debugf("no service details for %v: %v", application.Name(), err)
		}
		scale := application.GetScale()
		processedStatus.Scale = &scale
	}

	processedStatus.EndpointBindings = context.allAppsUnitsCharmBindings.endpointBindings[application.Name()]

	return processedStatus
}

func (context *statusContext) processRemoteApplications() map[string]params.RemoteApplicationStatus {
	applicationsMap := make(map[string]params.RemoteApplicationStatus)
	for _, app := range context.consumerRemoteApplications {
		applicationsMap[app.Name()] = context.processRemoteApplication(app)
	}
	return applicationsMap
}

func (context *statusContext) processRemoteApplication(application *state.RemoteApplication) (status params.RemoteApplicationStatus) {
	status.OfferURL, _ = application.URL()
	status.OfferName = application.Name()
	eps, err := application.Endpoints()
	if err != nil {
		status.Err = err
		return
	}
	status.Endpoints = make([]params.RemoteEndpoint, len(eps))
	for i, ep := range eps {
		status.Endpoints[i] = params.RemoteEndpoint{
			Name:      ep.Name,
			Interface: ep.Interface,
			Role:      ep.Role,
		}
	}
	status.Life = processLife(application)

	status.Relations, err = context.processRemoteApplicationRelations(application)
	if err != nil {
		status.Err = err
		return
	}
	applicationStatus, err := application.Status()
	populateStatusFromStatusInfoAndErr(&status.Status, applicationStatus, err)
	return status
}

type offerStatus struct {
	crossmodel.ApplicationOffer
	err                  error
	charmURL             string
	activeConnectedCount int
	totalConnectedCount  int
}

func (context *statusContext) processOffers() map[string]params.ApplicationOfferStatus {
	offers := make(map[string]params.ApplicationOfferStatus)
	for name, offer := range context.offers {
		offerStatus := params.ApplicationOfferStatus{
			Err:                  offer.err,
			ApplicationName:      offer.ApplicationName,
			OfferName:            offer.OfferName,
			CharmURL:             offer.charmURL,
			Endpoints:            make(map[string]params.RemoteEndpoint),
			ActiveConnectedCount: offer.activeConnectedCount,
			TotalConnectedCount:  offer.totalConnectedCount,
		}
		for name, ep := range offer.Endpoints {
			offerStatus.Endpoints[name] = params.RemoteEndpoint{
				Name:      ep.Name,
				Interface: ep.Interface,
				Role:      ep.Role,
			}
		}
		offers[name] = offerStatus
	}
	return offers
}

func isColorStatus(code state.MeterStatusCode) bool {
	return code == state.MeterGreen || code == state.MeterAmber || code == state.MeterRed
}

func (context *statusContext) processUnitMeterStatuses(units map[string]*state.Unit) map[string]params.MeterStatus {
	unitsMap := make(map[string]params.MeterStatus)
	for _, unit := range units {
		meterStatus, err := unit.GetMeterStatus()
		if err != nil {
			continue
		}
		if isColorStatus(meterStatus.Code) {
			unitsMap[unit.Name()] = params.MeterStatus{Color: strings.ToLower(meterStatus.Code.String()), Message: meterStatus.Info}
		}
	}
	if len(unitsMap) > 0 {
		return unitsMap
	}
	return nil
}

func (context *statusContext) processUnits(units map[string]*state.Unit, applicationCharm string) map[string]params.UnitStatus {
	unitsMap := make(map[string]params.UnitStatus)
	for _, unit := range units {
		unitsMap[unit.Name()] = context.processUnit(unit, applicationCharm)
	}
	return unitsMap
}

func (context *statusContext) processUnit(unit *state.Unit, applicationCharm string) params.UnitStatus {
	var result params.UnitStatus
	if unit.ShouldBeAssigned() {
		addr, err := unit.PublicAddress()
		if err != nil {
			// Usually this indicates that no addresses have been set on the
			// machine yet.
			addr = network.Address{}
			logger.Debugf("error fetching public address: %v", err)
		}
		result.PublicAddress = addr.Value
	} else {
		// For CAAS units we want to provide the container address.
		container, err := unit.ContainerInfo()
		if err == nil {
			addr := container.Address()
			if addr != nil {
				result.Address = addr.Value
			}
		} else {
			logger.Debugf("error fetching container address: %v", err)
		}
	}
	unitPorts, _ := unit.OpenedPorts()
	for _, port := range unitPorts {
		result.OpenedPorts = append(result.OpenedPorts, port.String())
	}
	if unit.IsPrincipal() {
		result.Machine, _ = unit.AssignedMachineId()
	}
	curl, _ := unit.CharmURL()
	if applicationCharm != "" && curl != nil && curl.String() != applicationCharm {
		result.Charm = curl.String()
	}
	workloadVersion, err := context.status.UnitWorkloadVersion(unit.Name())
	if err == nil {
		result.WorkloadVersion = workloadVersion
	} else {
		logger.Debugf("error fetching workload version: %v", err)
	}

	result.AgentStatus, result.WorkloadStatus = context.processUnitAndAgentStatus(unit)

	if subUnits := unit.SubordinateNames(); len(subUnits) > 0 {
		result.Subordinates = make(map[string]params.UnitStatus)
		for _, name := range subUnits {
			subUnit := context.unitByName(name)
			// subUnit may be nil if subordinate was filtered out.
			if subUnit != nil {
				result.Subordinates[name] = context.processUnit(subUnit, applicationCharm)
			}
		}
	}
	if leader := context.leaders[unit.ApplicationName()]; leader == unit.Name() {
		result.Leader = true
	}
	containerInfo, err := unit.ContainerInfo()
	if err != nil && !errors.IsNotFound(err) {
		logger.Debugf("error fetching container info: %v", err)
	} else if err == nil {
		result.ProviderId = containerInfo.ProviderId()
		addr := containerInfo.Address()
		if addr != nil {
			result.Address = addr.Value
		}

		if len(result.OpenedPorts) == 0 {
			result.OpenedPorts = containerInfo.Ports()
		}
	}
	return result
}

func (context *statusContext) unitByName(name string) *state.Unit {
	applicationName := strings.Split(name, "/")[0]
	return context.allAppsUnitsCharmBindings.units[applicationName][name]
}

func (context *statusContext) processApplicationRelations(application *state.Application) (related map[string][]string, subord []string, err error) {
	subordSet := make(set.Strings)
	related = make(map[string][]string)
	relations := context.relations[application.Name()]
	for _, relation := range relations {
		ep, err := relation.Endpoint(application.Name())
		if err != nil {
			return nil, nil, err
		}
		relationName := ep.Relation.Name
		eps, err := relation.RelatedEndpoints(application.Name())
		if err != nil {
			return nil, nil, err
		}
		for _, ep := range eps {
			if isSubordinate(&ep, application) {
				subordSet.Add(ep.ApplicationName)
			}
			related[relationName] = append(related[relationName], ep.ApplicationName)
		}
	}
	for relationName, applicationNames := range related {
		sn := set.NewStrings(applicationNames...)
		related[relationName] = sn.SortedValues()
	}
	return related, subordSet.SortedValues(), nil
}

func (context *statusContext) processRemoteApplicationRelations(application *state.RemoteApplication) (related map[string][]string, err error) {
	related = make(map[string][]string)
	relations := context.relations[application.Name()]
	for _, relation := range relations {
		ep, err := relation.Endpoint(application.Name())
		if err != nil {
			return nil, err
		}
		relationName := ep.Relation.Name
		eps, err := relation.RelatedEndpoints(application.Name())
		if err != nil {
			return nil, err
		}
		for _, ep := range eps {
			related[relationName] = append(related[relationName], ep.ApplicationName)
		}
	}
	for relationName, applicationNames := range related {
		sn := set.NewStrings(applicationNames...)
		related[relationName] = sn.SortedValues()
	}
	return related, nil
}

type lifer interface {
	Life() state.Life
}

// contextUnit overloads the AgentStatus and Status calls to use the cached
// status values, and delegates everything else to the Unit.
// TODO: cache presence as well.
type contextUnit struct {
	*state.Unit
	context *statusContext
}

// AgentStatus implements UnitStatusGetter.
func (c *contextUnit) AgentStatus() (status.StatusInfo, error) {
	return c.context.status.UnitAgent(c.Name())
}

// Status implements UnitStatusGetter.
func (c *contextUnit) Status() (status.StatusInfo, error) {
	return c.context.status.UnitWorkload(c.Name())
}

// processUnitAndAgentStatus retrieves status information for both unit and unitAgents.
func (c *statusContext) processUnitAndAgentStatus(unit *state.Unit) (agentStatus, workloadStatus params.DetailedStatus) {
	wrapped := &contextUnit{unit, c}
	agent, workload := c.presence.UnitStatus(wrapped)
	populateStatusFromStatusInfoAndErr(&agentStatus, agent.Status, agent.Err)
	populateStatusFromStatusInfoAndErr(&workloadStatus, workload.Status, workload.Err)

	agentStatus.Life = processLife(unit)

	if t, err := unit.AgentTools(); err == nil {
		agentStatus.Version = t.Version.Number.String()
	}
	return
}

// populateStatusFromStatusInfoAndErr creates AgentStatus from the typical output
// of a status getter.
// TODO: make this a function that just returns a type.
func populateStatusFromStatusInfoAndErr(agent *params.DetailedStatus, statusInfo status.StatusInfo, err error) {
	agent.Err = err
	agent.Status = statusInfo.Status.String()
	agent.Info = statusInfo.Message
	agent.Data = filterStatusData(statusInfo.Data)
	agent.Since = statusInfo.Since
}

// contextMachine overloads the Status call to use the cached status values,
// and delegates everything else to the Machine.
// TODO: cache presence as well.
type contextMachine struct {
	*state.Machine
	context *statusContext
}

// Return the agent status for the machine.
func (c *contextMachine) Status() (status.StatusInfo, error) {
	return c.context.status.MachineAgent(c.Id())
}

// processMachine retrieves version and status information for the given machine.
// It also returns deprecated legacy status information.
func (c *statusContext) processMachine(machine *state.Machine) (out params.DetailedStatus) {
	wrapped := &contextMachine{machine, c}
	statusInfo, err := c.presence.MachineStatus(wrapped)
	populateStatusFromStatusInfoAndErr(&out, statusInfo, err)

	out.Life = processLife(machine)

	if t, err := machine.AgentTools(); err == nil {
		out.Version = t.Version.Number.String()
	}
	return
}

// filterStatusData limits what agent StatusData data is passed over
// the API. This prevents unintended leakage of internal-only data.
func filterStatusData(status map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for name, value := range status {
		// use a set here if we end up with a larger whitelist
		if name == "relation-id" {
			out[name] = value
		}
	}
	return out
}

func processLife(entity lifer) string {
	if life := entity.Life(); life != state.Alive {
		// alive is the usual state so omit it by default.
		return life.String()
	}
	return ""
}

type bySinceDescending []status.StatusInfo

// Len implements sort.Interface.
func (s bySinceDescending) Len() int { return len(s) }

// Swap implements sort.Interface.
func (s bySinceDescending) Swap(a, b int) { s[a], s[b] = s[b], s[a] }

// Less implements sort.Interface.
func (s bySinceDescending) Less(a, b int) bool { return s[a].Since.After(*s[b].Since) }
