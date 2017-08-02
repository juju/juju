// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/status"
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
	var err error
	if context.applications, context.units, context.latestCharms, err =
		fetchAllApplicationsAndUnits(c.api.stateAccessor, len(args.Patterns) <= 0); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch applications and units")
	}
	if featureflag.Enabled(feature.CrossModelRelations) {
		if context.consumerRemoteApplications, err =
			fetchConsumerRemoteApplications(c.api.stateAccessor); err != nil {
			return noStatus, errors.Annotate(err, "could not fetch remote applications")
		}
		// Only admins can see offer details.
		if err := c.checkIsAdmin(); err == nil {
			if context.offerConnections, err =
				fetchOfferConnections(c.api.stateAccessor); err != nil {
				return noStatus, errors.Annotate(err, "could not fetch offer connections")
			}
			if context.offers, err =
				fetchOffers(c.api.stateAccessor); err != nil {
				return noStatus, errors.Annotate(err, "could not fetch application offers")
			}
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
	if len(context.applications) > 0 {
		if context.leaders, err = c.api.stateAccessor.ApplicationLeaders(); err != nil {
			return noStatus, errors.Annotate(err, " could not fetch leaders")
		}
	}

	logger.Debugf("Applications: %v", context.applications)
	logger.Debugf("Remote applications: %v", context.consumerRemoteApplications)
	logger.Debugf("Offers: %v", context.offers)
	logger.Debugf("Offer connections: %v", context.offerConnections)

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
		for _, unitMap := range context.units {
			for name, unit := range unitMap {
				machineId, err := unit.AssignedMachineId()
				if err != nil {
					machineId = ""
				} else if matchedMachines.Contains(machineId) {
					// Unit is on a matching machine.
					matchedSvcs.Add(unit.ApplicationName())
					continue
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

		// Filter applications
		for svcName, svc := range context.applications {
			if matchedSvcs.Contains(svcName) {
				// There are matched units for this application.
				continue
			} else if matches, err := predicate(svc); err != nil {
				return noStatus, errors.Annotate(err, "could not filter applications")
			} else if !matches {
				delete(context.applications, svcName)
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
		Model: modelStatus,
		Machines: processMachines(
			context.machines,
			context.ipAddresses,
			context.spaces,
			context.linkLayerDevices,
		),
		Applications:       context.processApplications(),
		RemoteApplications: context.processRemoteApplications(),
		Offers:             context.processOffers(),
		Relations:          context.processRelations(),
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
	ms := m.MeterStatus()
	if isColorStatus(ms.Code) {
		info.MeterStatus = params.MeterStatus{Color: strings.ToLower(ms.Code.String()), Message: ms.Info}
	}

	return info, nil
}

type statusContext struct {
	// machines: top-level machine id -> list of machines nested in
	// this machine.
	machines map[string][]*state.Machine

	// ipAddresses: machine id -> list of ip.addresses
	ipAddresses map[string][]*state.Address

	// spaces: machine id -> deviceName -> list of spaceNames
	spaces map[string]map[string]set.Strings

	// linkLayerDevices: machine id -> list of linkLayerDevices
	linkLayerDevices map[string][]*state.LinkLayerDevice

	// applications: application name -> application
	applications map[string]*state.Application

	// remote applications: application name -> application
	consumerRemoteApplications map[string]*state.RemoteApplication

	// offers: offer name -> offer
	offers map[string]offerStatus

	// offerConnections: relationId -> offer connection
	offerConnections map[int]*state.OfferConnection

	relations     map[string][]*state.Relation
	relationsById map[int]*state.Relation
	units         map[string]map[string]*state.Unit
	latestCharms  map[charm.URL]*state.Charm
	leaders       map[string]string
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
		subnet, err := st.Subnet(ipAddr.SubnetCIDR())
		if errors.IsNotFound(err) {
			// No worries; no subnets means no spaces.
			continue
		} else if err != nil {
			return nil, nil, nil, err
		}
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

	linkLayerDevices := make(map[string][]*state.LinkLayerDevice)
	llDevs, err := st.AllLinkLayerDevices()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, llDev := range llDevs {
		if llDev.IsLoopbackDevice() {
			continue
		}
		addrs, err := llDev.Addresses()
		if err != nil {
			return nil, nil, nil, err
		}
		// We don't want to see bond slaves or bridge ports, only the
		// IP-addressed devices.
		if len(addrs) > 0 {
			machineID := llDev.MachineID()
			linkLayerDevices[machineID] = append(linkLayerDevices[machineID], llDev)
		}
	}

	return ipAddresses, spaces, linkLayerDevices, nil
}

// fetchAllApplicationsAndUnits returns a map from application name to application,
// a map from application name to unit name to unit, and a map from base charm URL to latest URL.
func fetchAllApplicationsAndUnits(
	st Backend,
	matchAny bool,
) (map[string]*state.Application, map[string]map[string]*state.Unit, map[charm.URL]*state.Charm, error) {

	appMap := make(map[string]*state.Application)
	unitMap := make(map[string]map[string]*state.Unit)
	latestCharms := make(map[charm.URL]*state.Charm)
	applications, err := st.AllApplications()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, s := range applications {
		units, err := s.AllUnits()
		if err != nil {
			return nil, nil, nil, err
		}
		appUnitMap := make(map[string]*state.Unit)
		for _, u := range units {
			appUnitMap[u.Name()] = u
		}
		if matchAny || len(appUnitMap) > 0 {
			unitMap[s.Name()] = appUnitMap
			appMap[s.Name()] = s
			// Record the base URL for the application's charm so that
			// the latest store revision can be looked up.
			charmURL, _ := s.CharmURL()
			if charmURL.Schema == "cs" {
				latestCharms[*charmURL.WithRevision(-1)] = nil
			}
		}
	}
	for baseURL := range latestCharms {
		ch, err := st.LatestPlaceholderCharm(&baseURL)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, nil, nil, err
		}
		latestCharms[baseURL] = ch
	}

	return appMap, unitMap, latestCharms, nil
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
func fetchOffers(st Backend) (map[string]offerStatus, error) {
	offersMap := make(map[string]offerStatus)
	offers, err := st.AllApplicationOffers()
	if err != nil {
		return nil, err
	}
	model, err := st.Model()
	if err != nil {
		return nil, err
	}
	for _, offer := range offers {
		offersMap[offer.OfferName] = offerStatus{
			ApplicationOffer: crossmodel.ApplicationOffer{
				OfferName:       offer.OfferName,
				ApplicationName: offer.ApplicationName,
				Endpoints:       offer.Endpoints,
			},
			ApplicationURL: crossmodel.MakeURL(model.Owner().Name(), model.Name(), offer.OfferName, ""),
		}
	}
	return offersMap, nil
}

// fetchOfferConnections returns a map from relation id to offer connection.
func fetchOfferConnections(st Backend) (map[int]*state.OfferConnection, error) {
	connMap := make(map[int]*state.OfferConnection)
	conns, err := st.AllOfferConnections()
	if err != nil {
		return nil, err
	}
	for _, c := range conns {
		connMap[c.RelationId()] = c
	}
	return connMap, nil
}

// fetchRelations returns a map of all relations keyed by application name.
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

func processMachines(
	idToMachines map[string][]*state.Machine,
	idToIpAddresses map[string][]*state.Address,
	idToDeviceToSpaces map[string]map[string]set.Strings,
	idToLinkLayerDevices map[string][]*state.LinkLayerDevice,
) map[string]params.MachineStatus {
	machinesMap := make(map[string]params.MachineStatus)
	cache := make(map[string]params.MachineStatus)
	for id, machines := range idToMachines {

		if len(machines) <= 0 {
			continue
		}

		// Element 0 is assumed to be the top-level machine.
		tlMachine := machines[0]
		hostStatus := makeMachineStatus(
			tlMachine,
			idToIpAddresses[tlMachine.Id()],
			idToDeviceToSpaces[tlMachine.Id()],
			idToLinkLayerDevices[tlMachine.Id()],
		)
		machinesMap[id] = hostStatus
		cache[id] = hostStatus

		for _, machine := range machines[1:] {
			parent, ok := cache[state.ParentId(machine.Id())]
			if !ok {
				panic("We've broken an assumpution.")
			}

			status := makeMachineStatus(
				machine,
				idToIpAddresses[machine.Id()],
				idToDeviceToSpaces[machine.Id()],
				idToLinkLayerDevices[machine.Id()],
			)
			parent.Containers[machine.Id()] = status
			cache[machine.Id()] = status
		}
	}
	return machinesMap
}

func makeMachineStatus(
	machine *state.Machine,
	ipAddresses []*state.Address,
	spaces map[string]set.Strings,
	linkLayerDevices []*state.LinkLayerDevice,
) (status params.MachineStatus) {
	var err error
	status.Id = machine.Id()
	agentStatus := processMachine(machine)
	status.AgentStatus = agentStatus

	status.Series = machine.Series()
	status.Jobs = paramsJobsFromJobs(machine.Jobs())
	status.WantsVote = machine.WantsVote()
	status.HasVote = machine.HasVote()
	sInfo, err := machine.InstanceStatus()
	populateStatusFromStatusInfoAndErr(&status.InstanceStatus, sInfo, err)
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
		logger.Debugf("NetworkInterfaces: %+v", status.NetworkInterfaces)
	} else {
		if errors.IsNotProvisioned(err) {
			status.InstanceId = "pending"
		} else {
			status.InstanceId = "error"
		}
	}
	constraints, err := machine.Constraints()
	if err != nil {
		if !errors.IsNotFound(err) {
			status.Constraints = "error"
		}
	} else {
		status.Constraints = constraints.String()
	}
	hc, err := machine.HardwareCharacteristics()
	if err != nil {
		if !errors.IsNotFound(err) {
			status.Hardware = "error"
		}
	} else {
		status.Hardware = hc.String()
	}
	status.Containers = make(map[string]params.MachineStatus)
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
	application := context.applications[ep.ApplicationName]
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
	for _, s := range context.applications {
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
		Charm:   applicationCharm.URL().String(),
		Series:  application.Series(),
		Exposed: application.IsExposed(),
		Life:    processLife(application),
	}

	if latestCharm, ok := context.latestCharms[*applicationCharm.URL().WithRevision(-1)]; ok && latestCharm != nil {
		if latestCharm.Revision() > applicationCharm.URL().Revision {
			processedStatus.CanUpgradeTo = latestCharm.String()
		}
	}

	processedStatus.Relations, processedStatus.SubordinateTo, err = context.processApplicationRelations(application)
	if err != nil {
		processedStatus.Err = common.ServerError(err)
		return processedStatus
	}
	units := context.units[application.Name()]
	if application.IsPrincipal() {
		processedStatus.Units = context.processUnits(units, applicationCharm.URL().String())
	}
	applicationStatus, err := application.Status()
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

	versions := make([]status.StatusInfo, 0, len(units))
	for _, unit := range units {
		statuses, err := unit.WorkloadVersionHistory().StatusHistory(
			status.StatusHistoryFilter{Size: 1},
		)
		if err != nil {
			processedStatus.Err = common.ServerError(err)
			return processedStatus
		}
		// Even though we fully expect there to be historical values there,
		// even the first should be the empty string, the status history
		// collection is not added to in a transactional manner, so it may be
		// not there even though we'd really like it to be. Such is mongo.
		if len(statuses) > 0 {
			versions = append(versions, statuses[0])
		}
	}
	if len(versions) > 0 {
		sort.Sort(bySinceDescending(versions))
		processedStatus.WorkloadVersion = versions[0].Message
	}

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
	status.ApplicationURL, _ = application.URL()
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
			Scope:     ep.Scope,
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
	ApplicationURL string
}

func (context *statusContext) processOffers() map[string]params.ApplicationOfferStatus {
	offers := make(map[string]params.ApplicationOfferStatus)
	for name, offer := range context.offers {
		offerStatus := params.ApplicationOfferStatus{
			ApplicationName: offer.ApplicationName,
			OfferName:       offer.OfferName,
			ApplicationURL:  offer.ApplicationURL,
			Endpoints:       make(map[string]params.RemoteEndpoint),
			Connections:     make(map[int]params.OfferConnectionStatus),
		}
		for name, ep := range offer.Endpoints {
			offerStatus.Endpoints[name] = params.RemoteEndpoint{
				Name:      ep.Name,
				Interface: ep.Interface,
				Role:      ep.Role,
				Limit:     ep.Limit,
			}
		}
		offers[name] = offerStatus
	}
	for relId, conn := range context.offerConnections {
		connStatus := context.processOfferConnection(conn)
		offer, ok := offers[conn.OfferName()]
		if !ok {
			continue
		}
		offer.Connections[relId] = connStatus
		offers[conn.OfferName()] = offer
	}
	return offers
}

func (context *statusContext) processOfferConnection(conn *state.OfferConnection) (status params.OfferConnectionStatus) {
	status.SourceModelTag = names.NewModelTag(conn.SourceModelUUID()).String()
	status.Username = conn.UserName()
	rel, ok := context.relationsById[conn.RelationId()]
	if !ok {
		status.Err = errors.NotFoundf("relation id %d", conn.RelationId())
		return
	}
	offer, ok := context.offers[conn.OfferName()]
	if !ok {
		status.Err = errors.NotFoundf("application offer %s", conn.OfferName())
		return
	}
	ep, err := rel.Endpoint(offer.ApplicationName)
	if err != nil {
		status.Err = err
		return
	}
	status.Endpoint = ep.Name
	// TODO(wallyworld)
	status.Status = "active"
	return status
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
	addr, err := unit.PublicAddress()
	if err != nil {
		// Usually this indicates that no addresses have been set on the
		// machine yet.
		addr = network.Address{}
		logger.Debugf("error fetching public address: %v", err)
	}
	result.PublicAddress = addr.Value
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
	workloadVersion, err := unit.WorkloadVersion()
	if err == nil {
		result.WorkloadVersion = workloadVersion
	} else {
		logger.Debugf("error fetching workload version: %v", err)
	}

	processUnitAndAgentStatus(unit, &result)

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
	return result
}

func (context *statusContext) unitByName(name string) *state.Unit {
	applicationName := strings.Split(name, "/")[0]
	return context.units[applicationName][name]
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

// processUnitAndAgentStatus retrieves status information for both unit and unitAgents.
func processUnitAndAgentStatus(unit *state.Unit, unitStatus *params.UnitStatus) {
	unitStatus.AgentStatus, unitStatus.WorkloadStatus = processUnit(unit)
}

// populateStatusFromStatusInfoAndErr creates AgentStatus from the typical output
// of a status getter.
func populateStatusFromStatusInfoAndErr(agent *params.DetailedStatus, statusInfo status.StatusInfo, err error) {
	agent.Err = err
	agent.Status = statusInfo.Status.String()
	agent.Info = statusInfo.Message
	agent.Data = filterStatusData(statusInfo.Data)
	agent.Since = statusInfo.Since
}

// processMachine retrieves version and status information for the given machine.
// It also returns deprecated legacy status information.
func processMachine(machine *state.Machine) (out params.DetailedStatus) {
	statusInfo, err := common.MachineStatus(machine)
	populateStatusFromStatusInfoAndErr(&out, statusInfo, err)

	out.Life = processLife(machine)

	if t, err := machine.AgentTools(); err == nil {
		out.Version = t.Version.Number.String()
	}
	return
}

// processUnit retrieves version and status information for the given unit.
func processUnit(unit *state.Unit) (agentStatus, workloadStatus params.DetailedStatus) {
	agent, workload := common.UnitStatus(unit)
	populateStatusFromStatusInfoAndErr(&agentStatus, agent.Status, agent.Err)
	populateStatusFromStatusInfoAndErr(&workloadStatus, workload.Status, workload.Err)

	agentStatus.Life = processLife(unit)

	if t, err := unit.AgentTools(); err == nil {
		agentStatus.Version = t.Version.Number.String()
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
