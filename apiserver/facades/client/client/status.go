// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"sort"
	"strings"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/core/cache"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreseries "github.com/juju/juju/core/series"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
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

// applicationStatusHistory returns status history for the given (remote) application.
func (c *Client) applicationStatusHistory(appTag names.ApplicationTag, filter status.StatusHistoryFilter,
	kind status.HistoryKind) ([]params.DetailedStatus, error) {
	var (
		app status.StatusHistoryGetter
		err error
	)
	if kind == status.KindApplication {
		app, err = c.api.stateAccessor.Application(appTag.Name)
	} else {
		app, err = c.api.stateAccessor.RemoteApplication(appTag.Name)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	sInfo, err := app.StatusHistory(filter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return agentStatusFromStatusInfo(sInfo, kind), nil
}

// unitStatusHistory returns a list of status history entries for unit agents or workloads.
func (c *Client) unitStatusHistory(unitTag names.UnitTag, filter status.StatusHistoryFilter,
	kind status.HistoryKind) ([]params.DetailedStatus, error) {
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
func (c *Client) machineStatusHistory(machineTag names.MachineTag, filter status.StatusHistoryFilter,
	kind status.HistoryKind) ([]params.DetailedStatus, error) {
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

// modelStatusHistory returns status history for the current model.
func (c *Client) modelStatusHistory(filter status.StatusHistoryFilter) ([]params.DetailedStatus, error) {
	m, err := c.api.stateAccessor.Model()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get model")
	}

	sInfo, err := m.StatusHistory(filter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return agentStatusFromStatusInfo(sInfo, status.KindModel), nil
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
				Error: apiservererrors.ServerError(err),
			}
			results.Results = append(results.Results, history)
			continue

		}

		if err := filter.Validate(); err != nil {
			history := params.StatusHistoryResult{
				Error: apiservererrors.ServerError(errors.Annotate(err, "cannot validate status history filter")),
			}
			results.Results = append(results.Results, history)
			continue
		}

		var (
			err  error
			hist []params.DetailedStatus
		)
		kind := status.HistoryKind(request.Kind)
		switch kind {
		case status.KindModel:
			hist, err = c.modelStatusHistory(filter)
		case status.KindUnit, status.KindWorkload, status.KindUnitAgent:
			var u names.UnitTag
			if u, err = names.ParseUnitTag(request.Tag); err == nil {
				hist, err = c.unitStatusHistory(u, filter, kind)
			}
		case status.KindApplication, status.KindSAAS:
			var app names.ApplicationTag
			if app, err = names.ParseApplicationTag(request.Tag); err == nil {
				hist, err = c.applicationStatusHistory(app, filter, kind)
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
				Error: apiservererrors.ServerError(errors.Annotatef(err, "fetching status history for %q",
					request.Tag)),
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
	context.cachedModel = c.api.modelCache
	context.appCharmCache = map[string]string{}

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

	if context.spaceInfos, err = c.api.stateAccessor.AllSpaceInfos(); err != nil {
		return noStatus, errors.Annotate(err, "cannot obtain space information")
	}
	if context.model, err = c.api.state().Model(); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch model")
	}
	if context.status, err = context.model.LoadModelStatus(); err != nil {
		return noStatus, errors.Annotate(err, "could not load model status values")
	}
	if context.allAppsUnitsCharmBindings, err =
		fetchAllApplicationsAndUnits(c.api.stateAccessor, context.model, context.spaceInfos); err != nil {
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
	if err = context.fetchMachines(c.api.stateAccessor); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch machines")
	}
	if err = context.fetchOpenPortRangesForAllMachines(c.api.stateAccessor); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch open port ranges")
	}
	if context.controllerNodes, err = fetchControllerNodes(c.api.stateAccessor); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch controller nodes")
	}
	if len(context.controllerNodes) > 1 {
		if primaryHAMachine, err := c.api.stateAccessor.HAPrimaryMachine(); err != nil {
			// We do not want to return any errors here as they are all
			// non-fatal for this call since we can still
			// get FullStatus including machine info even if we could not get HA Primary determined.
			// Also on some non-HA setups, i.e. where mongo was not run with --replSet,
			// this call will return an error.
			logger.Warningf("could not determine if there is a primary HA machine: %v", err)
		} else {
			context.primaryHAMachine = &primaryHAMachine
		}
	}
	// These may be empty when machines have not finished deployment.
	if context.ipAddresses, context.spaces, context.linkLayerDevices, err =
		fetchNetworkInterfaces(c.api.stateAccessor, context.spaceInfos); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch IP addresses and link layer devices")
	}
	if context.relations, context.relationsById, err = fetchRelations(c.api.stateAccessor); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch relations")
	}
	if len(context.allAppsUnitsCharmBindings.applications) > 0 {
		if context.leaders, err = c.api.leadershipReader.Leaders(); err != nil {
			return noStatus, errors.Annotate(err, "could not fetch leaders")
		}
	}
	if context.controllerTimestamp, err = c.api.stateAccessor.ControllerTimestamp(); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch controller timestamp")
	}
	context.branches = fetchBranches(c.api.modelCache)

	logger.Tracef("Applications: %v", context.allAppsUnitsCharmBindings.applications)
	logger.Tracef("Remote applications: %v", context.consumerRemoteApplications)
	logger.Tracef("Offers: %v", context.offers)
	logger.Tracef("Relations: %v", context.relations)

	if len(args.Patterns) > 0 {
		patterns := resolveLeaderUnits(args.Patterns, context.leaders)
		predicate := BuildPredicateFor(patterns)

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
		matchedApps := set.NewStrings()
		matchedUnits := set.NewStrings()
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
					matchedApps.Add(unit.ApplicationName())
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
				matchedApps.Add(unit.ApplicationName())
				matchedUnits.Add(unit.Name())
				matchedUnits = matchedUnits.Union(set.NewStrings(unit.SubordinateNames()...))
				if machineId != "" {
					matchedMachines.Add(machineId)
				}
			}
		}
		for _, m := range matchedMachines.SortedValues() {
			for _, a := range machineUnits[m] {
				if !matchedApps.Contains(a) {
					matchedApps.Add(a)
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
			if !matchedApps.Contains(appName) && !matches {
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
		for aStatus, machineList := range context.machines {
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
			context.machines[aStatus] = matched
		}

		// Filter branches
		context.branches = filterBranches(context.branches, matchedApps,
			matchedUnits.Union(set.NewStrings(args.Patterns...)))
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
		Branches:            context.processBranches(),
	}, nil
}

// resolveLeaderUnits resolves the passed in leader pattern to an existing application leader unit
// and then replaces it inplace in the patterns
func resolveLeaderUnits(patterns []string, leaders map[string]string) []string {
	for i, v := range patterns {
		if strings.Contains(v, "leader") {
			application := strings.Split(v, "/")[0]
			unit, ok := leaders[application]
			if ok {
				patterns[i] = unit
				continue
			}
		}
	}
	return patterns
}

func filterBranches(ctxBranches map[string]cache.Branch,
	matchedApps, matchedForBranches set.Strings) map[string]cache.Branch {
	// Filter branches based on matchedApps which contains
	// the application name if matching on application or unit.
	unmatchedBranches := set.NewStrings()
	// Need a combination of the pattern strings and all units
	// matched above, both principal and subordinate.
	for bName, branch := range ctxBranches {
		unmatchedBranches.Add(bName)
		for appName, units := range branch.AssignedUnits() {
			appMatch := matchedForBranches.Contains(appName)
			// if the application is in the pattern, and this
			// branch,
			contains := matchedApps.Contains(appName)
			if contains && appMatch {
				unmatchedBranches.Remove(bName)
				break
			}
			// if the application is in this branch, but not
			// the pattern, check if any assigned units are in
			// the pattern
			if contains && !appMatch {
				for _, u := range units {
					if matchedForBranches.Contains(u) {
						unmatchedBranches.Remove(bName)
						break
					}
				}
			}
		}
	}
	for _, deleteBranch := range unmatchedBranches.Values() {
		delete(ctxBranches, deleteBranch)
	}
	return ctxBranches
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
	info.CloudTag = names.NewCloudTag(m.CloudName()).String()
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

	aStatus, err := m.Status()
	if err != nil {
		return params.ModelStatusInfo{}, errors.Annotate(err, "cannot obtain model status info")
	}

	info.SLA = m.SLALevel()

	info.ModelStatus = params.DetailedStatus{
		Status: aStatus.Status.String(),
		Info:   aStatus.Message,
		Since:  aStatus.Since,
		Data:   aStatus.Data,
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
	cachedModel  *cache.Model
	model        *state.Model
	status       *state.ModelStatus
	presence     common.ModelPresenceContext

	// machines: top-level machine id -> list of machines nested in
	// this machine.
	machines map[string][]*state.Machine
	// allMachines: machine id -> machine
	// The machine in this map is the same machine in the machines map.
	allMachines    map[string]*state.Machine
	allInstances   *state.ModelInstanceData
	allConstraints *state.ModelConstraints

	// controllerNodes: node id -> controller node
	controllerNodes map[string]state.ControllerNode

	// ipAddresses: machine id -> list of ip.addresses
	ipAddresses map[string][]*state.Address

	// spaces: machine id -> deviceName -> list of spaceNames
	spaces map[string]map[string]set.Strings

	// linkLayerDevices: machine id -> list of linkLayerDevices
	linkLayerDevices map[string][]*state.LinkLayerDevice

	// remote applications: application name -> application
	consumerRemoteApplications map[string]*state.RemoteApplication

	// opened ports by machine.
	openPortRangesByMachine map[string]state.MachinePortRanges

	// offers: offer name -> offer
	offers map[string]offerStatus

	// controller current timestamp
	controllerTimestamp *time.Time

	allAppsUnitsCharmBindings applicationStatusInfo
	relations                 map[string][]*state.Relation
	relationsById             map[int]*state.Relation
	leaders                   map[string]string
	branches                  map[string]cache.Branch

	// Information about all spaces.
	spaceInfos network.SpaceInfos

	primaryHAMachine *names.MachineTag

	// Cache the map from an application to its charm information
	appCharmCache map[string]string
}

// fetchMachines returns a map from top level machine id to machines, where machines[0] is the host
// machine and machines[1..n] are any containers (including nested ones).
//
// If machineIds is non-nil, only machines whose IDs are in the set are returned.
func (context *statusContext) fetchMachines(st Backend) error {
	if context.model.Type() == state.ModelTypeCAAS {
		return nil
	}
	context.machines = make(map[string][]*state.Machine)
	context.allMachines = make(map[string]*state.Machine)

	machines, err := st.AllMachines()
	if err != nil {
		return err
	}
	// AllMachines gives us machines sorted by id.
	for _, m := range machines {
		context.allMachines[m.Id()] = m
		_, ok := m.ParentId()
		if !ok {
			// Only top level host machines go directly into the machine map.
			context.machines[m.Id()] = []*state.Machine{m}
		} else {
			topParentId := container.TopParentId(m.Id())
			machines := context.machines[topParentId]
			context.machines[topParentId] = append(machines, m)
		}
	}

	context.allInstances, err = context.model.AllInstanceData()
	if err != nil {
		return err
	}
	context.allConstraints, err = context.model.AllConstraints()
	if err != nil {
		return err
	}

	return nil
}

func (context *statusContext) fetchOpenPortRangesForAllMachines(st Backend) error {
	if context.model.Type() == state.ModelTypeCAAS {
		return nil
	}

	context.openPortRangesByMachine = make(map[string]state.MachinePortRanges)
	allMachPortRanges, err := context.model.OpenedPortRangesForAllMachines()
	if err != nil {
		return err
	}
	for _, machPortRanges := range allMachPortRanges {
		context.openPortRangesByMachine[machPortRanges.MachineID()] = machPortRanges
	}
	return nil
}

// fetchControllerNodes returns a map from node id to controller node.
func fetchControllerNodes(st Backend) (map[string]state.ControllerNode, error) {
	v := make(map[string]state.ControllerNode)
	nodes, err := st.ControllerNodes()
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		v[n.Id()] = n
	}
	return v, nil
}

// fetchNetworkInterfaces returns maps from machine id to ip.addresses, machine
// id to a map of interface names from space names, and machine id to
// linklayerdevices.
//
// All are required to determine a machine's network interfaces configuration,
// so we want all or none.
func fetchNetworkInterfaces(st Backend, spaceInfos network.SpaceInfos) (map[string][]*state.Address,
	map[string]map[string]set.Strings, map[string][]*state.LinkLayerDevice, error) {
	ipAddresses := make(map[string][]*state.Address)
	spacesPerMachine := make(map[string]map[string]set.Strings)
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
			spaceName := network.AlphaSpaceName
			spaceInfo := spaceInfos.GetByID(subnet.SpaceID())
			if spaceInfo != nil {
				spaceName = string(spaceInfo.Name)
			}
			if spaceName != "" {
				devices, ok := spacesPerMachine[machineID]
				if !ok {
					devices = make(map[string]set.Strings)
					spacesPerMachine[machineID] = devices
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

	return ipAddresses, spacesPerMachine, linkLayerDevices, nil
}

// fetchAllApplicationsAndUnits returns a map from application name to application,
// a map from application name to unit name to unit, and a map from base charm URL to latest URL.
func fetchAllApplicationsAndUnits(st Backend, model *state.Model, spaceInfos network.SpaceInfos) (applicationStatusInfo,
	error) {
	appMap := make(map[string]*state.Application)
	unitMap := make(map[string]map[string]*state.Unit)
	latestCharms := make(map[charm.URL]*state.Charm)
	applications, err := st.AllApplications()
	if err != nil {
		return applicationStatusInfo{}, err
	}
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
	for app, bindings := range endpointBindings {
		// If the only binding is the default, and it's set to the
		// default space, no need to print.
		bindingMap, err := bindings.MapWithSpaceNames(spaceInfos)
		if err != nil {
			return applicationStatusInfo{}, err
		}
		if len(bindingMap) == 1 {
			if v, ok := bindingMap[""]; ok && v == network.AlphaSpaceName {
				continue
			}
		}
		allBindingsByApp[app] = bindingMap
	}

	lxdProfiles := make(map[string]*charm.LXDProfile)
	for _, app := range applications {
		appMap[app.Name()] = app
		appUnits := allUnitsByApp[app.Name()]
		cURL, _ := app.CharmURL()
		charmURL, err := charm.ParseURL(*cURL)
		if err != nil {
			continue
		}
		if len(appUnits) > 0 {
			unitMap[app.Name()] = appUnits
			// Record the base URL for the application's charm so that
			// the latest store revision can be looked up.
			switch {
			case charm.CharmHub.Matches(charmURL.Schema), charm.CharmStore.Matches(charmURL.Schema):
				latestCharms[*charmURL.WithRevision(-1)] = nil
			default:
				// Don't look up revision for local charms
			}
		}

		ch, _, err := app.Charm()
		if err != nil {
			continue
		}
		chName := lxdprofile.Name(model.Name(), app.Name(), ch.Revision())
		if profile := ch.LXDProfile(); profile != nil {
			lxdProfiles[chName] = &charm.LXDProfile{
				Description: profile.Description,
				Config:      profile.Config,
				Devices:     profile.Devices,
			}
		}
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
		if curl == nil {
			offerInfo.err = errors.NotValidf("application charm url nil")
			continue
		}
		offerInfo.charmURL = *curl
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

func fetchBranches(m *cache.Model) map[string]cache.Branch {
	// Unless you're using the generations feature flag,
	// the model cache model will be nil.  See note in
	// newFacade().
	if m == nil {
		return make(map[string]cache.Branch)
	}
	// m.Branches() returns only active branches.
	b := m.Branches()
	branches := make(map[string]cache.Branch, len(b))
	for _, branch := range b {
		branches[branch.Name()] = branch
	}
	return branches
}

func (c *statusContext) processMachines() map[string]params.MachineStatus {
	machinesMap := make(map[string]params.MachineStatus)
	aCache := make(map[string]params.MachineStatus)
	for id, machines := range c.machines {

		if len(machines) <= 0 {
			continue
		}

		// Element 0 is assumed to be the top-level machine.
		tlMachine := machines[0]
		hostStatus := c.makeMachineStatus(tlMachine, c.allAppsUnitsCharmBindings)
		machinesMap[id] = hostStatus
		aCache[id] = hostStatus

		for _, machine := range machines[1:] {
			parent, ok := aCache[container.ParentId(machine.Id())]
			if !ok {
				logger.Errorf("programmer error, please file a bug, reference this whole log line: %q, %q", id,
					machine.Id())
				continue
			}

			aStatus := c.makeMachineStatus(machine, c.allAppsUnitsCharmBindings)
			parent.Containers[machine.Id()] = aStatus
			aCache[machine.Id()] = aStatus
		}
	}
	return machinesMap
}

func (c *statusContext) makeMachineStatus(machine *state.Machine,
	appStatusInfo applicationStatusInfo) (status params.MachineStatus) {
	machineID := machine.Id()
	ipAddresses := c.ipAddresses[machineID]
	spaces := c.spaces[machineID]
	linkLayerDevices := c.linkLayerDevices[machineID]

	var err error
	status.Id = machineID
	agentStatus := c.processMachine(machine)
	status.AgentStatus = agentStatus

	status.Series = machine.Series()
	base, err := coreseries.GetBaseFromSeries(status.Series)
	if err != nil {
		logger.Errorf("cannot construct machine base from series %q", status.Series) //should never happen
	}
	status.Base = params.Base{Name: base.Name, Channel: base.Channel.String()}
	status.Jobs = paramsJobsFromJobs(machine.Jobs())
	node, wantsVote := c.controllerNodes[machineID]
	status.WantsVote = wantsVote
	if wantsVote {
		status.HasVote = node.HasVote()
	}
	if c.primaryHAMachine != nil {
		if isPrimary := c.primaryHAMachine.Id() == machineID; isPrimary {
			status.PrimaryControllerMachine = &isPrimary
		}
	}

	// Fetch the machine instance status information
	sInstInfo, err := c.status.MachineInstance(machineID)
	populateStatusFromStatusInfoAndErr(&status.InstanceStatus, sInstInfo, err)

	// Fetch the machine modification status information
	sModInfo, err := c.status.MachineModification(machineID)
	populateStatusFromStatusInfoAndErr(&status.ModificationStatus, sModInfo, err)

	instid, displayName := c.allInstances.InstanceNames(machineID)
	if instid != "" {
		status.InstanceId = instid
		status.DisplayName = displayName
		addr, err := machine.PublicAddress()
		if err != nil {
			// Usually this indicates that no addresses have been set on the
			// machine yet.
			addr = network.SpaceAddress{}
			logger.Debugf("error fetching public address: %q", err)
		}
		status.DNSName = addr.Value
		status.Hostname = machine.Hostname()
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
		status.InstanceId = "pending"
	}

	constraints := c.allConstraints.Machine(machineID)
	status.Constraints = constraints.String()

	hc := c.allInstances.HardwareCharacteristics(machineID)
	if hc != nil {
		status.Hardware = hc.String()
	}
	status.Containers = make(map[string]params.MachineStatus)

	lxdProfiles := make(map[string]params.LXDProfile)
	charmProfiles := c.allInstances.CharmProfiles(machineID)
	if charmProfiles != nil {
		for _, v := range charmProfiles {
			if profile, ok := appStatusInfo.lxdProfiles[v]; ok {
				lxdProfiles[v] = params.LXDProfile{
					Config:      profile.Config,
					Description: profile.Description,
					Devices:     profile.Devices,
				}
			}
		}
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
func paramsJobsFromJobs(jobs []state.MachineJob) []model.MachineJob {
	paramsJobs := make([]model.MachineJob, len(jobs))
	for i, machineJob := range jobs {
		paramsJobs[i] = machineJob.ToParams()
	}
	return paramsJobs
}

func (context *statusContext) processApplications() map[string]params.ApplicationStatus {
	applicationsMap := make(map[string]params.ApplicationStatus)
	for _, app := range context.allAppsUnitsCharmBindings.applications {
		applicationsMap[app.Name()] = context.processApplication(app)
	}
	return applicationsMap
}

func (context *statusContext) processApplication(application *state.Application) params.ApplicationStatus {
	applicationCharm, _, err := application.Charm()
	if err != nil {
		return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}
	}

	var charmProfileName string
	if lxdprofile.NotEmpty(lxdStateCharmProfiler{
		Charm: applicationCharm,
	}) {
		charmProfileName = lxdprofile.Name(context.model.Name(), application.Name(), applicationCharm.Revision())
	}

	mappedExposedEndpoints, err := context.mapExposedEndpointsFromState(application.ExposedEndpoints())
	if err != nil {
		return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}
	}

	var channel string
	if origin := application.CharmOrigin(); origin != nil && origin.Channel != nil {
		stChannel := origin.Channel
		channel = (charm.Channel{
			Track:  stChannel.Track,
			Risk:   charm.Risk(stChannel.Risk),
			Branch: stChannel.Branch,
		}).Normalize().String()
	} else {
		channel = string(application.Channel())
	}

	appSeries := application.Series()
	// Sidecar k8s charms have the appSeries set to that of the underlying base.
	// We want to ensure they are still shown as "kubernetes" in status.
	// TODO(juju3) - we want to reflect the underlying base, so remove this
	if corecharm.IsKubernetes(applicationCharm) {
		appSeries = coreseries.Kubernetes.String()
	}
	origin := application.CharmOrigin()
	if appSeries == "" && origin != nil && origin.Platform != nil {
		appSeries = origin.Platform.Series
	}
	var base coreseries.Base
	if appSeries != "" {
		base, err = coreseries.GetBaseFromSeries(appSeries)
		if err != nil {
			return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}
		}
	}

	var processedStatus = params.ApplicationStatus{
		Charm:        applicationCharm.String(),
		CharmVersion: applicationCharm.Version(),
		CharmProfile: charmProfileName,
		CharmChannel: channel,
		Series:       appSeries,
		Base: params.Base{
			Name:    base.Name,
			Channel: base.Channel.String(),
		},
		Exposed:          application.IsExposed(),
		ExposedEndpoints: mappedExposedEndpoints,
		Life:             processLife(application),
	}

	if latestCharm, ok := context.allAppsUnitsCharmBindings.latestCharms[*applicationCharm.URL().WithRevision(-1)]; ok && latestCharm != nil {
		if latestCharm.Revision() > applicationCharm.URL().Revision {
			processedStatus.CanUpgradeTo = latestCharm.String()
		}
	}

	processedStatus.Relations, processedStatus.SubordinateTo, err = context.processApplicationRelations(application)
	if err != nil {
		processedStatus.Err = apiservererrors.ServerError(err)
		return processedStatus
	}
	units := context.allAppsUnitsCharmBindings.units[application.Name()]
	if application.IsPrincipal() {
		expectWorkload, err := state.CheckApplicationExpectsWorkload(context.model, application.Name())
		if err != nil {
			return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}
		}
		processedStatus.Units = context.processUnits(units, applicationCharm.String(), expectWorkload)
	}

	// If for whatever reason the application isn't yet in the cache,
	// we have an unknown status.
	applicationStatus := status.StatusInfo{Status: status.Unknown}
	cachedApp, err := context.cachedModel.Application(application.Name())
	if err == nil {
		applicationStatus = cachedApp.DisplayStatus()
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
		workloadVersion, err := context.status.FullUnitWorkloadVersion(unit.Name())
		if err != nil {
			processedStatus.Err = apiservererrors.ServerError(err)
			return processedStatus
		}
		versions = append(versions, workloadVersion)
	}
	if len(versions) > 0 {
		sort.Sort(bySinceDescending(versions))
		processedStatus.WorkloadVersion = versions[0].Message
	}

	if processedStatus.WorkloadVersion == "" && context.model.Type() == state.ModelTypeCAAS {
		// We'll punt on using the docker image name.
		caasModel, err := context.model.CAASModel()
		if err != nil {
			return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}
		}
		specStr, err := caasModel.PodSpec(application.ApplicationTag())
		if err != nil && !errors.IsNotFound(err) {
			return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}
		}
		// TODO(caas): get WorkloadVersion from rawSpec once `ParseRawK8sSpec` is implemented.
		if specStr != "" {
			spec, err := k8sspecs.ParsePodSpec(specStr)
			if err != nil {
				return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}
			}
			// Container zero is the primary.
			primary := spec.Containers[0]
			processedStatus.WorkloadVersion = primary.ImageDetails.ImagePath
			if processedStatus.WorkloadVersion == "" {
				processedStatus.WorkloadVersion = spec.Containers[0].Image
			}
		}
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
	processedStatus.Scale = application.GetScale()
	processedStatus.EndpointBindings = context.allAppsUnitsCharmBindings.endpointBindings[application.Name()]
	return processedStatus
}

func (context *statusContext) mapExposedEndpointsFromState(exposedEndpoints map[string]state.ExposedEndpoint) (map[string]params.ExposedEndpoint,
	error) {
	if len(exposedEndpoints) == 0 {
		return nil, nil
	}

	res := make(map[string]params.ExposedEndpoint, len(exposedEndpoints))
	for endpointName, exposeDetails := range exposedEndpoints {
		mappedParam := params.ExposedEndpoint{
			ExposeToCIDRs: exposeDetails.ExposeToCIDRs,
		}

		if len(exposeDetails.ExposeToSpaceIDs) != 0 {
			spaceNames := make([]string, len(exposeDetails.ExposeToSpaceIDs))
			for i, spaceID := range exposeDetails.ExposeToSpaceIDs {
				sp := context.spaceInfos.GetByID(spaceID)
				if sp == nil {
					return nil, errors.NotFoundf("space with ID %q", spaceID)
				}

				spaceNames[i] = string(sp.Name)
			}
			mappedParam.ExposeToSpaces = spaceNames
		}

		res[endpointName] = mappedParam
	}

	return res, nil
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
		status.Err = apiservererrors.ServerError(err)
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
		status.Err = apiservererrors.ServerError(err)
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
			Err:                  apiservererrors.ServerError(offer.err),
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
			unitsMap[unit.Name()] = params.MeterStatus{Color: strings.ToLower(meterStatus.Code.String()),
				Message: meterStatus.Info}
		}
	}
	if len(unitsMap) > 0 {
		return unitsMap
	}
	return nil
}

func (context *statusContext) processUnits(units map[string]*state.Unit, applicationCharm string,
	expectWorkload bool) map[string]params.UnitStatus {
	unitsMap := make(map[string]params.UnitStatus)
	for _, unit := range units {
		unitsMap[unit.Name()] = context.processUnit(unit, applicationCharm, expectWorkload)
	}
	return unitsMap
}

func (context *statusContext) getAppCharm(unit *state.Unit) string {
	appName := unit.ApplicationName()
	charmStr, ok := context.appCharmCache[appName]
	if ok {
		return charmStr
	}
	app, err := unit.Application()
	if err != nil {
		logger.Debugf("error fetching subordinate application for %q: %q", appName, err.Error())
		context.appCharmCache[appName] = ""
		return ""
	}
	appCharm, _, err := app.Charm()
	if err != nil {
		logger.Debugf("error fetching subordinate application charm for %q: %q", appName, err.Error())
		context.appCharmCache[appName] = ""
		return ""
	}
	charmStr = appCharm.String()
	context.appCharmCache[appName] = charmStr
	return charmStr
}

func (context *statusContext) unitMachineID(unit *state.Unit) string {
	// This should never happen, but guarding against segfaults if for
	// some reason the unit isn't in the context.
	if unit == nil {
		return ""
	}
	principal, isSubordinate := unit.PrincipalName()
	if isSubordinate {
		return context.unitMachineID(context.unitByName(principal))
	}
	// machineID will be empty if not currently assigned.
	machineID, _ := unit.AssignedMachineId()
	return machineID
}

func (context *statusContext) unitPublicAddress(unit *state.Unit) string {
	machine := context.allMachines[context.unitMachineID(unit)]
	if machine == nil {
		return ""
	}
	// We don't care if the machine doesn't have an address yet.
	addr, _ := machine.PublicAddress()
	return addr.Value
}

func (context *statusContext) processUnit(unit *state.Unit, applicationCharm string,
	expectWorkload bool) params.UnitStatus {
	var result params.UnitStatus
	if context.model.Type() == state.ModelTypeIAAS {
		result.PublicAddress = context.unitPublicAddress(unit)

		if machPortRanges, found := context.openPortRangesByMachine[context.unitMachineID(unit)]; found {
			for _, pr := range machPortRanges.ForUnit(unit.Name()).UniquePortRanges() {
				result.OpenedPorts = append(result.OpenedPorts, pr.String())
			}
		}
	} else {
		// For CAAS units we want to provide the container address.
		// TODO: preload all the container info.
		container, err := unit.ContainerInfo()
		if err == nil {
			if addr := container.Address(); addr != nil {
				result.Address = addr.Value
			}
			result.ProviderId = container.ProviderId()
			if len(result.OpenedPorts) == 0 {
				result.OpenedPorts = container.Ports()
			}

		} else {
			logger.Tracef("container info not yet available for unit: %v", err)
		}
	}
	if unit.IsPrincipal() {
		result.Machine, _ = unit.AssignedMachineId()
	}
	unitCharm := unit.CharmURL()
	if applicationCharm != "" && unitCharm != nil && *unitCharm != applicationCharm {
		result.Charm = *unitCharm
	}
	workloadVersion, err := context.status.UnitWorkloadVersion(unit.Name())
	if err == nil {
		result.WorkloadVersion = workloadVersion
	} else {
		logger.Debugf("error fetching workload version: %v", err)
	}

	result.AgentStatus, result.WorkloadStatus = context.processUnitAndAgentStatus(unit, expectWorkload)

	if subUnits := unit.SubordinateNames(); len(subUnits) > 0 {
		result.Subordinates = make(map[string]params.UnitStatus)
		for _, name := range subUnits {
			subUnit := context.unitByName(name)
			// subUnit may be nil if subordinate was filtered out.
			if subUnit != nil {
				subUnitAppCharm := context.getAppCharm(subUnit)
				result.Subordinates[name] = context.processUnit(subUnit, subUnitAppCharm, true)
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
	return context.allAppsUnitsCharmBindings.units[applicationName][name]
}

func (context *statusContext) processApplicationRelations(application *state.Application) (related map[string][]string,
	subord []string, err error) {
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

func (context *statusContext) processRemoteApplicationRelations(application *state.RemoteApplication) (related map[string][]string,
	err error) {
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

func (c *statusContext) processBranches() map[string]params.BranchStatus {
	branchMap := make(map[string]params.BranchStatus, len(c.branches))
	for name, branch := range c.branches {
		branchMap[name] = params.BranchStatus{
			AssignedUnits: branch.AssignedUnits(),
			Created:       branch.Created(),
			CreatedBy:     branch.CreatedBy(),
		}
	}
	return branchMap
}

type lifer interface {
	Life() state.Life
}

// contextUnit overloads the AgentStatus and Status calls to use the cached
// status values, and delegates everything else to the Unit.
type contextUnit struct {
	*state.Unit
	expectWorkload bool
	context        *statusContext
}

// AgentStatus implements UnitStatusGetter.
func (c *contextUnit) AgentStatus() (status.StatusInfo, error) {
	return c.context.status.UnitAgent(c.Name())
}

// Status implements UnitStatusGetter.
func (c *contextUnit) Status() (status.StatusInfo, error) {
	return c.context.status.UnitWorkload(c.Name(), c.expectWorkload)
}

// processUnitAndAgentStatus retrieves status information for both unit and unitAgents.
func (c *statusContext) processUnitAndAgentStatus(unit *state.Unit,
	expectWorkload bool) (agentStatus, workloadStatus params.DetailedStatus) {
	wrapped := &contextUnit{unit, expectWorkload, c}
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
	agent.Err = apiservererrors.ServerError(err)
	agent.Status = statusInfo.Status.String()
	agent.Info = statusInfo.Message
	agent.Data = filterStatusData(statusInfo.Data)
	agent.Since = statusInfo.Since
}

// contextMachine overloads the Status call to use the cached status values,
// and delegates everything else to the Machine.
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

func processLife(entity lifer) life.Value {
	if aLife := entity.Life(); aLife != state.Alive {
		// alive is the usual state so omit it by default.
		return aLife.Value()
	}
	return life.Value("")
}

type bySinceDescending []status.StatusInfo

// Len implements sort.Interface.
func (s bySinceDescending) Len() int { return len(s) }

// Swap implements sort.Interface.
func (s bySinceDescending) Swap(a, b int) { s[a], s[b] = s[b], s[a] }

// Less implements sort.Interface.
func (s bySinceDescending) Less(a, b int) bool { return s[a].Since.After(*s[b].Since) }

// lxdStateCharmProfiler massages a *state.Charm into a LXDProfiler
// inside of the core package.
type lxdStateCharmProfiler struct {
	Charm *state.Charm
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p lxdStateCharmProfiler) LXDProfile() lxdprofile.LXDProfile {
	if p.Charm == nil {
		return nil
	}
	profile := p.Charm.LXDProfile()
	if profile == nil {
		return nil
	}
	return profile
}
