// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"

	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/internal/charms"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainmodelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/port"
	"github.com/juju/juju/domain/relation"
	statusservice "github.com/juju/juju/domain/status/service"
	"github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// StatusHistory returns a slice of past statuses for several entities.
func (c *Client) StatusHistory(ctx context.Context, request params.StatusHistoryRequests) params.StatusHistoryResults {
	return params.StatusHistoryResults{}
}

type lifer interface {
	Life() state.Life
}

// FullStatus gives the information needed for juju status over the api
func (c *Client) FullStatus(ctx context.Context, args params.StatusParams) (params.FullStatus, error) {
	if err := c.checkCanRead(ctx); err != nil {
		return params.FullStatus{}, err
	}

	if len(args.Patterns) > 0 {
		// Patterns have been disabled until we tackle the status epic. This
		// will require pushing the patterns down through the status service.
		// For now, just black hole the request.
		return params.FullStatus{}, internalerrors.Errorf("patterns are not implemented").Add(
			errors.NotImplemented,
		)
	}

	var noStatus params.FullStatus
	context := statusContext{
		applicationService: c.applicationService,
		statusService:      c.statusService,
	}

	var err error
	if context.model, err = c.modelInfoService.GetModelInfo(ctx); err != nil {
		return noStatus, fmt.Errorf("getting model info: %w", err)
	}
	context.providerType = context.model.CloudType

	if context.spaceInfos, err = c.networkService.GetAllSpaces(ctx); err != nil {
		return noStatus, internalerrors.Errorf("cannot obtain space information: %w", err)
	}
	if context.status, err = c.stateAccessor.AllStatus(); err != nil {
		return noStatus, internalerrors.Errorf("could not load model status values: %w", err)
	}
	if context.allAppsUnitsCharmBindings, err =
		fetchAllApplicationsAndUnits(ctx, c.statusService, c.applicationService, c.stateAccessor, context.spaceInfos); err != nil {
		return noStatus, internalerrors.Errorf("could not fetch applications and units: %w", err)
	}
	if context.consumerRemoteApplications, err =
		fetchConsumerRemoteApplications(c.stateAccessor); err != nil {
		return noStatus, internalerrors.Errorf("could not fetch remote applications: %w", err)
	}
	// Only admins can see offer details.
	if err := c.checkIsAdmin(ctx); err == nil {
		// TODO(gfouillet): Re-enable fetching for offer details once
		//   CMR will be moved in their own domain.
		logger.Warningf(ctx, "cross model relations are disabled until "+
			"backend functionality is moved to domain")
	}
	if err = context.fetchMachines(c.stateAccessor); err != nil {
		return noStatus, internalerrors.Errorf("could not fetch machines: %w", err)
	}
	if err = context.fetchAllOpenPortRanges(ctx, c.portService); err != nil {
		return noStatus, internalerrors.Errorf("could not fetch open port ranges: %w", err)
	}
	if context.controllerNodes, err = fetchControllerNodes(c.stateAccessor); err != nil {
		return noStatus, internalerrors.Errorf("could not fetch controller nodes: %w", err)
	}
	if len(context.controllerNodes) > 1 {
		if primaryHAMachine, err := c.stateAccessor.HAPrimaryMachine(); err != nil {
			// We do not want to return any errors here as they are all
			// non-fatal for this call since we can still
			// get FullStatus including machine info even if we could not get HA Primary determined.
			// Also on some non-HA setups, i.e. where mongo was not run with --replSet,
			// this call will return an error.
			logger.Warningf(ctx, "could not determine if there is a primary HA machine: %v", err)
		} else {
			context.primaryHAMachine = &primaryHAMachine
		}
	}
	// These may be empty when machines have not finished deployment.
	subnetInfos, err := c.networkService.GetAllSubnets(ctx)
	if err != nil {
		return noStatus, internalerrors.Errorf("could not fetch subnets: %w", err)
	}
	if context.ipAddresses, context.spaces, context.linkLayerDevices, err =
		fetchNetworkInterfaces(c.stateAccessor, subnetInfos, context.spaceInfos); err != nil {
		return noStatus, internalerrors.Errorf("could not fetch IP addresses and link layer devices: %w", err)
	}
	if context.relations, context.relationsByID, err = fetchRelations(ctx, c.relationService, c.statusService); err != nil {
		return noStatus, internalerrors.Errorf("could not fetch relations: %w", err)
	}
	if len(context.allAppsUnitsCharmBindings.applications) > 0 {
		if context.leaders, err = c.leadershipReader.Leaders(); err != nil {
			// Leader information is additive for status.
			// Given that it comes from Dqlite, which may be subject to
			// reconfiguration when mutating the control plane, we would
			// rather return as much status as possible over an error.
			logger.Warningf(ctx, "could not determine application leaders: %v", err)
			context.leaders = make(map[string]string)
		}
	}
	if context.controllerTimestamp, err = c.stateAccessor.ControllerTimestamp(); err != nil {
		return noStatus, internalerrors.Errorf("could not fetch controller timestamp: %w", err)
	}

	if args.IncludeStorage {
		context.storageInstances, err = c.storageAccessor.AllStorageInstances()
		if err != nil {
			return noStatus, internalerrors.Errorf("cannot list all storage instances: %w", err)
		}
		context.filesystems, err = c.storageAccessor.AllFilesystems()
		if err != nil {
			return noStatus, internalerrors.Errorf("cannot list all filesystems: %w", err)
		}
		context.volumes, err = c.storageAccessor.AllVolumes()
		if err != nil {
			return noStatus, internalerrors.Errorf("cannot list all volumes: %w", err)
		}
	}

	if logger.IsLevelEnabled(corelogger.TRACE) {
		logger.Tracef(ctx, "Applications: %v", context.allAppsUnitsCharmBindings.applications)
		logger.Tracef(ctx, "Remote applications: %v", context.consumerRemoteApplications)
		logger.Tracef(ctx, "Offers: %v", context.offers)
		logger.Tracef(ctx, "Leaders", context.leaders)
		logger.Tracef(ctx, "Relations: %v", context.relations)
		logger.Tracef(ctx, "StorageInstances: %v", context.storageInstances)
		logger.Tracef(ctx, "Filesystems: %v", context.filesystems)
		logger.Tracef(ctx, "Volumes: %v", context.volumes)
	}

	modelStatus, err := c.modelStatus(ctx)
	if err != nil {
		return noStatus, internalerrors.Errorf("cannot determine model status: %w", err)
	}

	var storageDetails []params.StorageDetails
	var filesystemDetails []params.FilesystemDetails
	var volumeDetails []params.VolumeDetails
	if args.IncludeStorage {
		storageDetails, err = context.processStorage(ctx, c.storageAccessor, c.blockDeviceService)
		if err != nil {
			return noStatus, internalerrors.Errorf("cannot process storage instances: %w", err)
		}
		filesystemDetails, err = context.processFilesystems(ctx, c.storageAccessor, c.blockDeviceService)
		if err != nil {
			return noStatus, internalerrors.Errorf("cannot process filesystems: %w", err)
		}
		volumeDetails, err = context.processVolumes(ctx, c.storageAccessor, c.blockDeviceService)
		if err != nil {
			return noStatus, internalerrors.Errorf("cannot process volumes: %w", err)
		}
	}

	return params.FullStatus{
		Model:               modelStatus,
		Machines:            context.processMachines(ctx, c.machineService),
		Applications:        context.processApplications(ctx),
		RemoteApplications:  context.processRemoteApplications(),
		Offers:              context.processOffers(),
		Relations:           context.processRelations(ctx),
		ControllerTimestamp: context.controllerTimestamp,
		Storage:             storageDetails,
		Filesystems:         filesystemDetails,
		Volumes:             volumeDetails,
	}, nil
}

// modelStatus returns the status of the current model.
func (c *Client) modelStatus(ctx context.Context) (params.ModelStatusInfo, error) {
	var info params.ModelStatusInfo

	modelInfo, err := c.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return info, fmt.Errorf("getting model info: %w", err)
	}
	info.Name = modelInfo.Name
	info.Type = modelInfo.Type.String()
	info.CloudTag = names.NewCloudTag(modelInfo.Cloud).String()
	info.CloudRegion = modelInfo.CloudRegion

	currentVersion := modelInfo.AgentVersion
	info.Version = currentVersion.String()

	// // TODO: replace here once we implement the latest agent version in Dqlite.
	// m, err := c.stateAccessor.Model()
	// if err != nil {
	// 	return info, internalerrors.Errorf("cannot get model: %w", err)
	// }

	// latestVersion := m.LatestToolsVersion()
	// if currentVersion.Compare(latestVersion) < 0 {
	// 	info.AvailableVersion = latestVersion.String()
	// }

	aStatus, err := c.modelInfoService.GetStatus(ctx)
	if internalerrors.Is(err, domainmodelerrors.NotFound) {
		// This should never happen but just in case.
		return params.ModelStatusInfo{}, internalerrors.Errorf("model status for %q: %w", modelInfo.Name, errors.NotFound)
	}
	if err != nil {
		return params.ModelStatusInfo{}, internalerrors.Errorf("cannot obtain model status info: %w", err)
	}

	info.ModelStatus = params.DetailedStatus{
		Status: aStatus.Status.String(),
		Info:   aStatus.Message,
		Since:  &aStatus.Since,
	}

	return info, nil
}

type applicationStatusInfo struct {
	// application: application name -> application
	applications map[string]statusservice.Application

	// units: application name -> units name -> units
	units map[string]map[string]*state.Unit

	// allUnits: unit name -> unit
	allUnits map[string]*state.Unit

	// endpointBindings: application name -> endpoint -> space
	endpointBindings map[string]map[string]string

	// latestCharms: charm URL -> charm locator
	latestCharms map[charm.URL]applicationcharm.CharmLocator

	// lxdProfiles: lxd profile name -> lxd profile
	lxdProfiles map[string]*charm.LXDProfile
}

type relationStatus struct {
	ID        int
	Key       corerelation.Key
	Endpoints []relation.Endpoint
	Status    status.StatusInfo
}

// Endpoint retrieves the relation endpoint associated with the specified application name from the relation status.
// Returns an error if the endpoint is not found.
func (s relationStatus) Endpoint(applicationName string) (relation.Endpoint, error) {
	for _, ep := range s.Endpoints {
		if ep.ApplicationName == applicationName {
			return ep, nil
		}
	}
	return relation.Endpoint{}, internalerrors.Errorf("endpoint for application %q: %w", applicationName, errors.NotFound)
}

// RelatedEndpoints returns the endpoints in the relation status that are related
// to the specified application.
// It filters endpoints based on the counterpart role of the specified
// application's endpoint role.
//
// We can have several relations by endpoint, either as providers or as
// requirers for different use case. An obvious one is a provider endpoint for
// a database. We can have several services using this database through this
// endpoint. Requirer endpoint with several provider are less obvious, but not
// prevented.
//
// Returns an error if the specified application's endpoint is not found or no related endpoints exist.
func (s relationStatus) RelatedEndpoints(applicationName string) ([]relation.Endpoint, error) {
	local, err := s.Endpoint(applicationName)
	if err != nil {
		return nil, err
	}
	role := relation.CounterpartRole(local.Role)
	var eps []relation.Endpoint
	for _, ep := range s.Endpoints {
		if ep.Role == role {
			eps = append(eps, ep)
		}
	}
	if eps == nil {
		return nil, internalerrors.Errorf("fetching endpoints of %q related to application %q: %w", s,
			applicationName, errors.NotFound)
	}
	return eps, nil
}

type statusContext struct {
	applicationService ApplicationService
	statusService      StatusService

	providerType string
	model        model.ModelInfo

	status *state.AllStatus

	// machines: top-level machine id -> list of machines nested in
	// this machine.
	machines map[string][]*state.Machine
	// allMachines: machine id -> machine
	// The machine in this map is the same machine in the machines map.
	allMachines        map[string]*state.Machine
	machineConstraints *state.MachineConstraints

	// controllerNodes: node id -> controller node
	controllerNodes map[string]state.ControllerNode

	// ipAddresses: machine id -> list of ip.addresses
	ipAddresses map[string][]*state.Address

	// spaces: machine id -> deviceName -> list of spaceNames
	spaces map[string]map[string]set.Strings

	// linkLayerDevices: machine id -> list of linkLayerDevices
	linkLayerDevices map[string][]*state.LinkLayerDevice

	// remote applications: application name -> application
	consumerRemoteApplications map[string]commoncrossmodel.RemoteApplication

	// allOpenPortRanges: all open port ranges in the model, grouped by unit name.
	allOpenPortRanges port.UnitGroupedPortRanges

	// offers: offer name -> offer
	offers map[string]offerStatus

	// controller current timestamp
	controllerTimestamp *time.Time

	allAppsUnitsCharmBindings applicationStatusInfo
	relations                 map[string][]relationStatus
	relationsByID             map[int]relationStatus
	leaders                   map[string]string

	// Information about all spaces.
	spaceInfos network.SpaceInfos

	primaryHAMachine *names.MachineTag

	// Optional storage info.
	storageInstances []state.StorageInstance
	volumes          []state.Volume
	filesystems      []state.Filesystem
}

// fetchMachines returns a map from top level machine id to machines, where machines[0] is the host
// machine and machines[1..n] are any containers (including nested ones).
//
// If machineIds is non-nil, only machines whose IDs are in the set are returned.
func (context *statusContext) fetchMachines(st Backend) error {
	if context.model.Type == model.CAAS {
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

	context.machineConstraints, err = st.MachineConstraints()
	if err != nil {
		return err
	}

	return nil
}

func (context *statusContext) fetchAllOpenPortRanges(ctx context.Context, portService PortService) error {
	var err error
	context.allOpenPortRanges, err = portService.GetAllOpenedPorts(ctx)
	return err
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
func fetchNetworkInterfaces(st Backend, subnetInfos network.SubnetInfos, spaceInfos network.SpaceInfos) (map[string][]*state.Address,
	map[string]map[string]set.Strings, map[string][]*state.LinkLayerDevice, error) {
	ipAddresses := make(map[string][]*state.Address)
	spacesPerMachine := make(map[string]map[string]set.Strings)
	subnetsByCIDR := make(map[string]network.SubnetInfo)
	for _, subnet := range subnetInfos {
		subnetsByCIDR[subnet.CIDR] = subnet
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
			spaceInfo := spaceInfos.GetByID(subnet.SpaceID)
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
func fetchAllApplicationsAndUnits(ctx context.Context, statusService StatusService, applicationService ApplicationService, st Backend, spaceInfos network.SpaceInfos) (applicationStatusInfo, error) {
	var (
		appMap       = make(map[string]statusservice.Application)
		unitMap      = make(map[string]map[string]*state.Unit)
		latestCharms = make(map[charm.URL]applicationcharm.CharmLocator)
	)

	applications, err := statusService.GetApplicationAndUnitStatuses(ctx)
	if err != nil {
		return applicationStatusInfo{}, err
	}
	units, err := st.AllUnits()
	if err != nil {
		return applicationStatusInfo{}, err
	}
	allUnitsByApp := make(map[string]map[string]*state.Unit)
	allUnits := make(map[string]*state.Unit)
	for _, unit := range units {
		appName := unit.ApplicationName()

		if inner, found := allUnitsByApp[appName]; found {
			inner[unit.Name()] = unit
		} else {
			allUnitsByApp[appName] = map[string]*state.Unit{
				unit.Name(): unit,
			}
		}

		allUnits[unit.Name()] = unit
	}

	endpointBindings, err := st.AllEndpointBindings()
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
	for name, app := range applications {
		appMap[name] = app
		appUnits := allUnitsByApp[name]
		if len(appUnits) > 0 {
			unitMap[name] = appUnits

			// Record the base URL for the application's charm so that
			// the latest store revision can be looked up.
			cURL, _ := charms.CharmURLFromLocator(app.CharmLocator.Name, app.CharmLocator)
			charmURL, err := charm.ParseURL(cURL)
			if err != nil {
				continue
			}

			// De-duplicate charms with the same name and architecture.
			switch {
			case charm.CharmHub.Matches(charmURL.Schema):
				latestCharms[*charmURL.WithRevision(-1)] = applicationcharm.CharmLocator{}
			default:
				// Don't look up revision for local charms
			}
		}
	}

	// Latest charm lookup for all base URLs.
	for baseURL := range latestCharms {
		locator, err := applicationService.GetLatestPendingCharmhubCharm(ctx, baseURL.Name, baseURL.Architecture)
		if internalerrors.Is(err, applicationerrors.CharmNotFound) {
			continue
		} else if err != nil {
			return applicationStatusInfo{}, err
		}

		latestCharms[baseURL] = locator
	}

	return applicationStatusInfo{
		applications:     appMap,
		units:            unitMap,
		allUnits:         allUnits,
		endpointBindings: allBindingsByApp,
		latestCharms:     latestCharms,
		lxdProfiles:      lxdProfiles,
	}, nil
}

// fetchConsumerRemoteApplications returns a map from application name to remote application.
func fetchConsumerRemoteApplications(st Backend) (map[string]commoncrossmodel.RemoteApplication, error) {
	appMap := make(map[string]commoncrossmodel.RemoteApplication)
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

// fetchRelations returns a map of all relations keyed by application name,
// and another map keyed by id.
//
// This structure is useful for processApplicationRelations() which needs
// to have the relations for each application. Reading them once here
// avoids the repeated DB hits to retrieve the relations for each
// application that used to happen in processApplicationRelations().
func fetchRelations(ctx context.Context, relationService RelationService,
	statusService StatusService) (map[string][]relationStatus,
	map[int]relationStatus, error) {
	details, err := relationService.GetAllRelationDetails(ctx)
	if err != nil {
		return nil, nil, internalerrors.Errorf("fetching relations: %w", err)
	}
	out := make(map[string][]relationStatus)
	outById := make(map[int]relationStatus)

	// If there are no details, just return empty maps without error to avoid an
	// useless call to the status service.
	if len(details) == 0 {
		return out, outById, nil
	}

	statuses, err := statusService.GetAllRelationStatuses(ctx)
	if err != nil {
		return nil, nil, internalerrors.Errorf("fetching relation statuses: %w", err)
	}
	// Protective code against nil map.
	if statuses == nil {
		statuses = make(map[corerelation.UUID]status.StatusInfo)
	}
	for _, detail := range details {
		var eids []corerelation.EndpointIdentifier
		for _, ep := range detail.Endpoints {
			eids = append(eids, ep.EndpointIdentifier())
		}
		key, err := corerelation.NewKey(eids)
		if err != nil {
			logger.Warningf(ctx, "failed to generate relation key for %q: %v", detail.UUID, err)
			continue
		}

		relStatus, ok := statuses[detail.UUID]
		if !ok {
			// This shouldn't happen, since a relation and its status are
			// supposed to be added in the same transaction.
			// However, if status command is run while removing a transaction, it
			// may happen.
			// It should be rare, and if it happens without above special
			// circumstance it could be due to a design decision, db slowness
			// or corrupted data, which would requires special attention.
			logger.Warningf(ctx, "no status for relation %d %q", detail.ID,
				key.String())
		}
		r := relationStatus{
			ID:        detail.ID,
			Endpoints: detail.Endpoints,
			Key:       key,
			Status:    relStatus,
		}
		outById[r.ID] = r
		for _, ep := range r.Endpoints {
			out[ep.ApplicationName] = append(out[ep.ApplicationName], r)
		}
	}
	return out, outById, nil
}

func (c *statusContext) processMachines(ctx context.Context, machineService MachineService) map[string]params.MachineStatus {
	machinesMap := make(map[string]params.MachineStatus)
	aCache := make(map[string]params.MachineStatus)
	for id, machines := range c.machines {

		if len(machines) <= 0 {
			continue
		}

		// Element 0 is assumed to be the top-level machine.
		tlMachine := machines[0]
		hostStatus := c.makeMachineStatus(ctx, tlMachine, machineService, c.allAppsUnitsCharmBindings)
		machinesMap[id] = hostStatus
		aCache[id] = hostStatus

		for _, machine := range machines[1:] {
			parent, ok := aCache[container.ParentId(machine.Id())]
			if !ok {
				logger.Errorf(ctx, "programmer error, please file a bug, reference this whole log line: %q, %q", id,
					machine.Id())
				continue
			}

			aStatus := c.makeMachineStatus(ctx, machine, machineService, c.allAppsUnitsCharmBindings)
			parent.Containers[machine.Id()] = aStatus
			aCache[machine.Id()] = aStatus
		}
	}
	return machinesMap
}

func (c *statusContext) makeMachineStatus(
	ctx context.Context,
	machine *state.Machine,
	machineService MachineService,
	appStatusInfo applicationStatusInfo,
) (status params.MachineStatus) {
	machineID := machine.Id()
	ipAddresses := c.ipAddresses[machineID]
	spaces := c.spaces[machineID]
	linkLayerDevices := c.linkLayerDevices[machineID]

	var err error
	status.Id = machineID
	agentStatus := c.processMachine(ctx, machine)
	status.AgentStatus = agentStatus

	mBase := machine.Base()
	status.Base = params.Base{Name: mBase.OS, Channel: mBase.Channel}
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

	var (
		instid      instance.Id
		displayName string
	)
	machineUUID, err := machineService.GetMachineUUID(ctx, coremachine.Name(machineID))
	if err != nil {
		logger.Debugf(ctx, "error retrieving uuid for machine: %q, %w", machineID, err)
	} else {
		instid, displayName, err = machineService.InstanceIDAndName(ctx, machineUUID)
		if err != nil && !internalerrors.Is(err, machineerrors.NotProvisioned) {
			logger.Debugf(ctx, "error retrieving instance ID and display name for machine: %q, %w", machineID, err)
		}
	}
	if instid != instance.UnknownId {
		status.InstanceId = instid
		status.DisplayName = displayName
		addr, err := machine.PublicAddress()
		if err != nil {
			// Usually this indicates that no addresses have been set on the
			// machine yet.
			addr = network.SpaceAddress{}
			logger.Debugf(ctx, "error fetching public address: %q", err)
		}
		status.DNSName = addr.Value
		status.Hostname = machine.Hostname()
		mAddrs := machine.Addresses()
		if len(mAddrs) == 0 {
			logger.Debugf(ctx, "no IP addresses fetched for machine %q", instid)
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
		logger.Tracef(ctx, "NetworkInterfaces: %+v", status.NetworkInterfaces)
	} else {
		status.InstanceId = "pending"
	}

	constraints := c.machineConstraints.Machine(machineID)
	status.Constraints = constraints.String()

	hc, err := machineService.HardwareCharacteristics(ctx, machineUUID)
	if internalerrors.Is(err, machineerrors.NotProvisioned) {
		logger.Debugf(ctx, "can't retrieve hardware characteristics of machine %q: not provisioned", machineUUID)
	}
	if err != nil {
		logger.Debugf(ctx, "error fetching hardware characteristics: %v", err)
	} else if hc != nil {
		status.Hardware = hc.String()
	}
	status.Containers = make(map[string]params.MachineStatus)

	lxdProfiles := make(map[string]params.LXDProfile)
	charmProfiles, err := machineService.AppliedLXDProfileNames(ctx, machineUUID)
	if internalerrors.Is(err, machineerrors.NotProvisioned) {
		logger.Debugf(ctx, "can't retrieve lxd profiles for machine %q: not provisioned", machineUUID)
	}
	if err != nil {
		logger.Debugf(ctx, "error fetching lxd profiles: %w", err)
	}

	for _, v := range charmProfiles {
		if profile, ok := appStatusInfo.lxdProfiles[v]; ok {
			lxdProfiles[v] = params.LXDProfile{
				Config:      profile.Config,
				Description: profile.Description,
				Devices:     profile.Devices,
			}
		}
	}

	status.LXDProfiles = lxdProfiles

	return
}

func (context *statusContext) processRelations(ctx context.Context) []params.RelationStatus {
	var out []params.RelationStatus
	for _, current := range context.relationsByID {
		var eps []params.EndpointStatus
		var scope charm.RelationScope
		var relationInterface string
		for _, ep := range current.Endpoints {
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
			Id:        current.ID,
			Key:       current.Key.String(),
			Interface: relationInterface,
			Scope:     string(scope),
			Endpoints: eps,
		}
		populateStatusFromStatusInfoAndErr(&relStatus.Status, current.Status, nil)
		out = append(out, relStatus)
	}
	return out
}

func (context *statusContext) isSubordinate(ep *relation.Endpoint) bool {
	application, ok := context.allAppsUnitsCharmBindings.applications[ep.ApplicationName]
	if !ok {
		return false
	}
	return isSubordinate(ep, application)
}

func isSubordinate(ep *relation.Endpoint, application statusservice.Application) bool {
	return ep.Scope == charm.ScopeContainer && application.Subordinate
}

// paramsJobsFromJobs converts state jobs to params jobs.
func paramsJobsFromJobs(jobs []state.MachineJob) []model.MachineJob {
	paramsJobs := make([]model.MachineJob, len(jobs))
	for i, machineJob := range jobs {
		paramsJobs[i] = machineJob.ToParams()
	}
	return paramsJobs
}

func (context *statusContext) processApplications(ctx context.Context) map[string]params.ApplicationStatus {
	applicationsMap := make(map[string]params.ApplicationStatus)
	for name, app := range context.allAppsUnitsCharmBindings.applications {
		applicationsMap[name] = context.processApplication(ctx, name, app)
	}
	return applicationsMap
}

func (context *statusContext) processApplicationExposedEndpoints(ctx context.Context, name string, application statusservice.Application) (map[string]params.ExposedEndpoint, error) {
	// If the application is not exposed, then we don't need to try and get the
	// exposed endpoints for the application. This reduces the number of default
	// calls to the application service.
	if !application.Exposed {
		return nil, nil
	}

	exposedEndpoints, err := context.applicationService.GetExposedEndpoints(ctx, name)
	if err != nil {
		return nil, err
	}
	return context.mapExposedEndpointsFromDomain(exposedEndpoints)
}

func (context *statusContext) processApplication(ctx context.Context, name string, application statusservice.Application) params.ApplicationStatus {
	exposedEndpoints, err := context.processApplicationExposedEndpoints(ctx, name, application)
	if err != nil {
		return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}

	}

	var channel string
	if ch := application.Channel; ch != nil {
		c := charm.Channel{
			Track:  ch.Track,
			Risk:   charm.Risk(ch.Risk),
			Branch: ch.Branch,
		}
		channel = c.Normalize().String()
	}

	base, err := encodePlatform(application.Platform)
	if err != nil {
		return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}
	}

	charmURL, err := charms.CharmURLFromLocator(application.CharmLocator.Name, application.CharmLocator)
	if err != nil {
		return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}
	}

	appStatus := application.Status
	processedStatus := params.ApplicationStatus{
		Charm:            charmURL,
		CharmVersion:     application.CharmVersion,
		CharmRev:         application.CharmLocator.Revision,
		CharmChannel:     channel,
		Base:             base,
		Exposed:          application.Exposed,
		ExposedEndpoints: exposedEndpoints,
		Life:             application.Life,
		Status: params.DetailedStatus{
			Status: appStatus.Status.String(),
			Info:   appStatus.Message,
			Data:   appStatus.Data,
			Since:  appStatus.Since,
		},
	}

	curl, err := charm.ParseURL(charmURL)
	if err != nil {
		return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}
	}
	if latestCharm, ok := context.allAppsUnitsCharmBindings.latestCharms[*curl.WithRevision(-1)]; ok && !latestCharm.IsZero() {
		processedStatus.CanUpgradeTo, err = charms.CharmURLFromLocator(latestCharm.Name, latestCharm)
		if err != nil {
			return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}
		}
	}

	processedStatus.Relations, processedStatus.SubordinateTo, err = context.processApplicationRelations(name, application)
	if err != nil {
		processedStatus.Err = apiservererrors.ServerError(err)
		return processedStatus
	}
	units := context.allAppsUnitsCharmBindings.units[name]
	if !application.Subordinate {
		processedStatus.Units = context.processUnits(ctx, units, charmURL)
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

	processedStatus.EndpointBindings = context.allAppsUnitsCharmBindings.endpointBindings[name]

	// IAAS applications have all the information they need in the application
	// status. CAAS applications have some additional information.
	if context.model.Type == model.IAAS {
		return processedStatus
	}

	// Handle CAAS applications fields independently of the IAAS ones.
	if providerID := application.K8sProviderID; providerID != nil {
		processedStatus.ProviderId = *providerID
		// TODO (stickupkid): Add addresses to the status for k8s applications.
	}

	if scale := application.Scale; scale != nil {
		processedStatus.Scale = *scale
	}

	return processedStatus
}

func (context *statusContext) mapExposedEndpointsFromDomain(
	exposedEndpoints map[string]application.ExposedEndpoint,
) (map[string]params.ExposedEndpoint, error) {
	if len(exposedEndpoints) == 0 {
		return nil, nil
	}

	res := make(map[string]params.ExposedEndpoint, len(exposedEndpoints))
	for endpointName, exposeDetails := range exposedEndpoints {
		mappedParam := params.ExposedEndpoint{
			ExposeToCIDRs: exposeDetails.ExposeToCIDRs.Values(),
		}

		if len(exposeDetails.ExposeToSpaceIDs) != 0 {
			spaceNames := make([]string, len(exposeDetails.ExposeToSpaceIDs))
			for i, spaceID := range exposeDetails.ExposeToSpaceIDs.Values() {
				sp := context.spaceInfos.GetByID(spaceID)
				if sp == nil {
					return nil, internalerrors.Errorf("space with ID %q: %w", spaceID, errors.NotFound)
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
		applicationsMap[app.Name()] = params.RemoteApplicationStatus{
			Err: apiservererrors.ServerError(internalerrors.Errorf("cross model relations are disabled until "+
				"backend functionality is moved to domain: %w", errors.NotImplemented)),
		}
	}
	return applicationsMap
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

func (context *statusContext) processUnits(ctx context.Context, units map[string]*state.Unit, applicationCharm string) map[string]params.UnitStatus {
	unitsMap := make(map[string]params.UnitStatus)
	for _, unit := range units {
		unitsMap[unit.Name()] = context.processUnit(ctx, unit, applicationCharm)
	}
	return unitsMap
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

func (context *statusContext) processUnit(ctx context.Context, unit *state.Unit, applicationCharm string) params.UnitStatus {
	var result params.UnitStatus
	// unit.Name was retrieved from Mongo, so we can trust it's valid.
	unitName := coreunit.Name(unit.Name())
	if prs, ok := context.allOpenPortRanges[unitName]; ok {
		result.OpenedPorts = transform.Slice(prs, network.PortRange.String)
	}
	if context.model.Type == model.IAAS {
		result.PublicAddress = context.unitPublicAddress(unit)
	} else {
		// For CAAS units we want to provide the container address.
		// TODO: preload all the container info.
		container, err := unit.ContainerInfo()
		if err == nil {
			if addr := container.Address(); addr != nil {
				result.Address = addr.Value
			}
			result.ProviderId = container.ProviderId()

		} else {
			logger.Tracef(ctx, "container info not yet available for unit: %v", err)
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
		logger.Debugf(ctx, "error fetching workload version: %v", err)
	}

	result.AgentStatus, result.WorkloadStatus = context.processUnitAndAgentStatus(ctx, unit, unitName)

	if subUnits := unit.SubordinateNames(); len(subUnits) > 0 {
		result.Subordinates = make(map[string]params.UnitStatus)
		for _, name := range subUnits {
			subUnit := context.unitByName(name)
			// subUnit may be nil if subordinate was filtered out.
			if subUnit != nil {
				subUnitAppCharm := ""
				subUnitApp, err := subUnit.Application()
				if err == nil {
					if subUnitAppCh, _, err := subUnitApp.Charm(); err == nil {
						subUnitAppCharm = subUnitAppCh.URL()
					} else {
						logger.Debugf(ctx, "error fetching subordinate application charm for %q: %q", subUnit.ApplicationName(), err.Error())
					}
				} else {
					// We can still run processUnit with an empty string for
					// the ApplicationCharm.
					logger.Debugf(ctx, "error fetching subordinate application for %q: %q", subUnit.ApplicationName(), err.Error())
				}
				result.Subordinates[name] = context.processUnit(ctx, subUnit, subUnitAppCharm)
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

func (context *statusContext) processApplicationRelations(
	name string,
	application statusservice.Application,
) (related map[string][]string, subord []string, err error) {
	subordSet := make(set.Strings)
	related = make(map[string][]string)
	relations := context.relations[name]
	for _, relation := range relations {
		ep, err := relation.Endpoint(name)
		if err != nil {
			return nil, nil, err
		}
		relationName := ep.Relation.Name
		eps, err := relation.RelatedEndpoints(name)
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

func (c *statusContext) unitToMachine(unitTag names.UnitTag) (names.MachineTag, error) {
	unit, ok := c.allAppsUnitsCharmBindings.allUnits[unitTag.Id()]
	if !ok {
		return names.MachineTag{}, internalerrors.Errorf("unit %v: %w", unitTag, errors.NotFound)
	}
	machine, err := unit.AssignedMachineId()
	if err != nil {
		return names.MachineTag{}, internalerrors.Capture(err)
	}
	return names.NewMachineTag(machine), nil
}

func (c *statusContext) processStorage(ctx context.Context, storageAccessor StorageInterface, blockDeviceService BlockDeviceService) ([]params.StorageDetails, error) {
	storageDetails := make([]params.StorageDetails, 0, len(c.storageInstances))
	for _, storageInstance := range c.storageInstances {
		storageDetail, err := storagecommon.StorageDetails(ctx, storageAccessor, blockDeviceService, c.unitToMachine, storageInstance)
		if err != nil {
			return nil, internalerrors.Errorf("cannot convert storage details for %v: %w", storageInstance.Tag(), err)
		}
		storageDetails = append(storageDetails, *storageDetail)
	}
	return storageDetails, nil
}

func (c *statusContext) processFilesystems(ctx context.Context, storageAccessor StorageInterface, blockDeviceService BlockDeviceService) ([]params.FilesystemDetails, error) {
	filesystemDetails := make([]params.FilesystemDetails, 0, len(c.filesystems))
	for _, filesystem := range c.filesystems {
		attachments, err := storageAccessor.FilesystemAttachments(filesystem.FilesystemTag())
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		filesystemDetail, err := storagecommon.FilesystemDetails(ctx, storageAccessor, blockDeviceService, c.unitToMachine, filesystem, attachments)
		if err != nil {
			return nil, internalerrors.Errorf("cannot convert filesystem details for %v: %w", filesystem.Tag(), err)
		}
		filesystemDetails = append(filesystemDetails, *filesystemDetail)
	}
	return filesystemDetails, nil
}

func (c *statusContext) processVolumes(ctx context.Context, storageAccessor StorageInterface, blockDeviceService BlockDeviceService) ([]params.VolumeDetails, error) {
	volumeDetails := make([]params.VolumeDetails, 0, len(c.volumes))
	for _, volume := range c.volumes {
		attachments, err := storageAccessor.VolumeAttachments(volume.VolumeTag())
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		volumeDetail, err := storagecommon.VolumeDetails(ctx, storageAccessor, blockDeviceService, c.unitToMachine, volume, attachments)
		if err != nil {
			return nil, internalerrors.Errorf("cannot convert volume details for %v: %w", volume.Tag(), err)
		}
		volumeDetails = append(volumeDetails, *volumeDetail)
	}
	return volumeDetails, nil
}

// processUnitAndAgentStatus retrieves status information for both unit and
// unitAgents.
//
// NOTE(jack-w-shaw): When this method was Mongo-backed, it will pull the unit
// statuses out of a cache.
func (c *statusContext) processUnitAndAgentStatus(ctx context.Context, unit *state.Unit, unitName coreunit.Name) (params.DetailedStatus, params.DetailedStatus) {
	agentStatus, workloadStatus, err := c.statusService.GetUnitDisplayAndAgentStatus(ctx, unitName)
	if internalerrors.Is(err, applicationerrors.UnitNotFound) {
		return params.DetailedStatus{}, params.DetailedStatus{Err: apiservererrors.ServerError(internalerrors.Errorf(
			"unit %q: %w", unitName, errors.NotFound))}
	} else if err != nil {
		return params.DetailedStatus{}, params.DetailedStatus{Err: apiservererrors.ServerError(err)}
	}

	detailedAgentStatus := params.DetailedStatus{
		Status: agentStatus.Status.String(),
		Info:   agentStatus.Message,
		Data:   filterStatusData(agentStatus.Data),
		Since:  agentStatus.Since,
		Life:   processLife(unit),
	}
	detailedWorkloadStatus := params.DetailedStatus{
		Status: workloadStatus.Status.String(),
		Info:   workloadStatus.Message,
		Data:   filterStatusData(workloadStatus.Data),
		Since:  workloadStatus.Since,
	}

	if t, err := unit.AgentTools(); err == nil {
		detailedAgentStatus.Version = t.Version.Number.String()
	}
	return detailedAgentStatus, detailedWorkloadStatus
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

// processMachine retrieves version and status information for the given machine.
// It also returns deprecated legacy status information.
func (c *statusContext) processMachine(ctx context.Context, machine *state.Machine) (out params.DetailedStatus) {
	statusInfo, err := machine.Status()
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

func encodePlatform(platform deployment.Platform) (params.Base, error) {
	os, err := encodeOSType(platform.OSType)
	if err != nil {
		return params.Base{}, err
	}

	base, err := base.ParseBase(os, platform.Channel)
	if err != nil {
		return params.Base{}, internalerrors.Errorf("parsing base %q: %w", os, err)
	}

	return params.Base{
		Name:    base.OS,
		Channel: base.Channel.String(),
	}, nil
}

func encodeOSType(ostype deployment.OSType) (string, error) {
	switch ostype {
	case deployment.Ubuntu:
		return base.UbuntuOS, nil
	default:
		return "", internalerrors.Errorf("unknown os type %q", ostype)
	}
}
