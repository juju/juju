// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"

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
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/port"
	"github.com/juju/juju/domain/relation"
	statusservice "github.com/juju/juju/domain/status/service"
	"github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// StatusHistory returns a slice of past statuses for several entities.
func (c *Client) StatusHistory(ctx context.Context, requests params.StatusHistoryRequests) params.StatusHistoryResults {
	if err := c.checkCanRead(ctx); err != nil {
		return statusHistoryResultsError(err, len(requests.Requests))
	}

	// This API officially supports bulk requests, but the client only sends
	// single requests. This prevents excessive memory usage in the server.
	if num := len(requests.Requests); num == 0 {
		return statusHistoryResultsError(nil, num)
	} else if num != 1 {
		return statusHistoryResultsError(internalerrors.Errorf("multiple requests are not supported"), num)
	}

	// We know we only have one request, so we can just use the first one.
	request := requests.Requests[0]

	kind := status.HistoryKind(request.Kind)
	if !kind.Valid() {
		return statusHistoryResultError(internalerrors.Errorf("invalid status history kind %q", request.Kind))
	}

	tag, err := names.ParseTag(request.Tag)
	if err != nil {
		return statusHistoryResultError(err)
	}

	history, err := c.statusService.GetStatusHistory(ctx, statusservice.StatusHistoryRequest{
		Kind: kind,
		Filter: statusservice.StatusHistoryFilter{
			Size:  request.Filter.Size,
			Date:  request.Filter.Date,
			Delta: request.Filter.Delta,
		},
		Tag: tag.Id(),
	})
	if err != nil {
		return statusHistoryResultError(err)
	}

	results := make([]params.DetailedStatus, len(history))
	for i, status := range history {
		results[i] = params.DetailedStatus{
			Status: status.Status.String(),
			Info:   status.Info,
			Since:  status.Since,
			Kind:   status.Kind.String(),
			Data:   status.Data,
		}
	}

	return params.StatusHistoryResults{
		Results: []params.StatusHistoryResult{{
			History: params.History{
				Statuses: results,
			},
		}},
	}
}

func statusHistoryResultsError(err error, amount int) params.StatusHistoryResults {
	results := make([]params.StatusHistoryResult, amount)
	for i := range results {
		results[i].Error = apiservererrors.ServerError(err)
	}
	return params.StatusHistoryResults{
		Results: results,
	}
}

func statusHistoryResultError(err error) params.StatusHistoryResults {
	return statusHistoryResultsError(err, 1)
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
		machineService:     c.machineService,
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
		fetchAllApplicationsAndUnits(ctx, c.statusService, c.applicationService); err != nil {
		return noStatus, internalerrors.Errorf("could not fetch applications and units: %w", err)
	}
	// Only admins can see offer details.
	if err := c.checkIsAdmin(ctx); err == nil {
		// TODO(gfouillet): Re-enable fetching for offer details once
		//   CMR will be moved in their own domain.
		logger.Tracef(ctx, "cross model relations are disabled until "+
			"backend functionality is moved to domain")
	}
	if err = context.fetchMachines(c.stateAccessor); err != nil {
		return noStatus, internalerrors.Errorf("could not fetch machines: %w", err)
	}
	if err = context.fetchAllOpenPortRanges(ctx, c.portService); err != nil {
		return noStatus, internalerrors.Errorf("could not fetch open port ranges: %w", err)
	}
	// These may be empty when machines have not finished deployment.
	if context.ipAddresses, context.linkLayerDevices, context.spaces, err = fetchNetworkInterfaces(ctx,
		c.networkService); err != nil {
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
		Machines:            context.processMachines(ctx),
		Applications:        context.processApplications(ctx),
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

	aStatus, err := c.statusService.GetModelStatus(ctx)
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
		Since:  aStatus.Since,
	}

	return info, nil
}

type applicationStatusInfo struct {
	// application: application name -> application
	applications map[string]statusservice.Application

	// applicationCharmURL holds the charm URL for a given application
	applicationCharmURL map[string]string

	// endpointBindings: application name -> endpoint -> space
	endpointBindings map[string]map[string]network.SpaceName

	// latestCharms: charm locator (without revision) -> charm locator
	latestCharms map[applicationcharm.CharmLocator]applicationcharm.CharmLocator

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
	machineService     MachineService

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

	// ipAddresses: machine id -> list of ip.addresses
	ipAddresses map[coremachine.Name][]domainnetwork.NetAddr

	// spaces: machine id -> deviceName -> list of spaceNames
	spaces map[coremachine.Name]map[string]set.Strings

	// linkLayerDevices: machine id -> list of linkLayerDevices
	linkLayerDevices map[coremachine.Name][]domainnetwork.NetInterface

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

	// Optional storage info.
	storageInstances []state.StorageInstance
	volumes          []state.Volume
	filesystems      []state.Filesystem
}

// fetchMachines returns a map from top level machine id to machines, where machines[0] is the host
// machine and machines[1..n] are any containers (including nested ones).
//
// If machineIds is non-nil, only machines whose IDs are in the set are returned.
func (c *statusContext) fetchMachines(st Backend) error {
	if c.model.Type == model.CAAS {
		return nil
	}
	c.machines = make(map[string][]*state.Machine)
	c.allMachines = make(map[string]*state.Machine)

	machines, err := st.AllMachines()
	if err != nil {
		return err
	}
	// AllMachines gives us machines sorted by id.
	for _, m := range machines {
		c.allMachines[m.Id()] = m
		_, ok := m.ParentId()
		if !ok {
			// Only top level host machines go directly into the machine map.
			c.machines[m.Id()] = []*state.Machine{m}
		} else {
			topParentId := container.TopParentId(m.Id())
			machines := c.machines[topParentId]
			c.machines[topParentId] = append(machines, m)
		}
	}

	c.machineConstraints, err = st.MachineConstraints()
	if err != nil {
		return err
	}

	return nil
}

func (c *statusContext) fetchAllOpenPortRanges(ctx context.Context, portService PortService) error {
	var err error
	c.allOpenPortRanges, err = portService.GetAllOpenedPorts(ctx)
	return err
}

func fetchNetworkInterfaces(
	ctx context.Context,
	networkService NetworkService,
) (
	map[coremachine.Name][]domainnetwork.NetAddr,
	map[coremachine.Name][]domainnetwork.NetInterface,
	map[coremachine.Name]map[string]set.Strings,
	error,
) {
	devices, err := networkService.GetAllDevicesByMachineNames(ctx)
	if err != nil {
		return nil, nil, nil, internalerrors.Errorf("fetching devices: %w", err)
	}

	// Remove loopback addresses
	devices = transform.Map(devices, func(k coremachine.Name, v []domainnetwork.NetInterface) (coremachine.
		Name,
		[]domainnetwork.NetInterface) {
		var filtered []domainnetwork.NetInterface
		for _, dev := range v {
			var nonLoopBack []domainnetwork.NetAddr
			for _, addr := range dev.Addrs {
				if addr.ConfigType == network.ConfigLoopback {
					continue
				}
				nonLoopBack = append(nonLoopBack, addr)
			}
			if len(nonLoopBack) > 0 {
				dev.Addrs = nonLoopBack
				filtered = append(filtered, dev)
			}
		}
		return k, filtered
	})

	ipAddresses := transform.Map(devices, func(k coremachine.Name,
		v []domainnetwork.NetInterface) (coremachine.Name, []domainnetwork.NetAddr) {
		var allAddresses []domainnetwork.NetAddr
		for _, dev := range v {
			allAddresses = append(allAddresses, dev.Addrs...)
		}
		return k, allAddresses
	})

	return ipAddresses, devices, nil, nil
}

// fetchAllApplicationsAndUnits returns a map from application name to application,
// a map from application name to unit name to unit, and a map from base charm URL to latest URL.
func fetchAllApplicationsAndUnits(ctx context.Context, statusService StatusService, applicationService ApplicationService) (applicationStatusInfo, error) {
	var (
		apps         = make(map[string]statusservice.Application)
		appCharmURL  = make(map[string]string)
		latestCharms = make(map[applicationcharm.CharmLocator]applicationcharm.CharmLocator)
	)

	applications, err := statusService.GetApplicationAndUnitStatuses(ctx)
	if err != nil {
		return applicationStatusInfo{}, err
	}

	allBindingsByApp, err := applicationService.GetAllEndpointBindings(ctx)
	if err != nil {
		return applicationStatusInfo{}, err
	}

	// If the only binding is the default, and it's set to the
	// default space, no need to print.
	for app, bindings := range allBindingsByApp {
		if len(bindings) == 1 {
			if v, ok := bindings[""]; ok && v == network.AlphaSpaceName {
				delete(allBindingsByApp, app)
			}
		}
	}

	lxdProfiles := make(map[string]*charm.LXDProfile)
	for name, app := range applications {
		apps[name] = app

		charmURL, err := charms.CharmURLFromLocator(app.CharmLocator.Name, app.CharmLocator)
		if err != nil {
			logger.Warningf(ctx, "failed to get charm URL for %q: %v", app.CharmLocator.Name, err)
			continue
		}
		appCharmURL[name] = charmURL

		if len(app.Units) == 0 {
			continue
		}

		// De-duplicate charms with the same name and architecture.
		// Don't look up revision for local charms
		if applicationcharm.CharmHubSource == app.CharmLocator.Source {
			latestCharms[app.CharmLocator.WithoutRevision()] = applicationcharm.CharmLocator{}
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
		applications:     apps,
		endpointBindings: allBindingsByApp,
		latestCharms:     latestCharms,
		lxdProfiles:      lxdProfiles,
	}, nil
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

func (c *statusContext) processMachines(ctx context.Context) map[string]params.MachineStatus {
	machinesMap := make(map[string]params.MachineStatus)
	aCache := make(map[string]params.MachineStatus)
	for id, machines := range c.machines {

		if len(machines) <= 0 {
			continue
		}

		// Element 0 is assumed to be the top-level machine.
		tlMachine := machines[0]
		hostStatus := c.makeMachineStatus(ctx, tlMachine, c.allAppsUnitsCharmBindings)
		machinesMap[id] = hostStatus
		aCache[id] = hostStatus

		for _, machine := range machines[1:] {
			parent, ok := aCache[container.ParentId(machine.Id())]
			if !ok {
				logger.Errorf(ctx, "programmer error, please file a bug, reference this whole log line: %q, %q", id,
					machine.Id())
				continue
			}

			aStatus := c.makeMachineStatus(ctx, machine, c.allAppsUnitsCharmBindings)
			parent.Containers[machine.Id()] = aStatus
			aCache[machine.Id()] = aStatus
		}
	}
	return machinesMap
}

func (c *statusContext) makeMachineStatus(
	ctx context.Context,
	machine *state.Machine,
	appStatusInfo applicationStatusInfo,
) (status params.MachineStatus) {
	machineID := machine.Id()

	var err error
	status.Id = machineID
	agentStatus := c.processMachine(ctx, machine)
	status.AgentStatus = agentStatus

	mBase := machine.Base()
	status.Base = params.Base{Name: mBase.OS, Channel: mBase.Channel}

	jobs := []model.MachineJob{model.JobHostUnits}
	if isController, err := c.machineService.IsMachineController(ctx, coremachine.Name(machineID)); err != nil && !stderrors.Is(err, machineerrors.MachineNotFound) {
		logger.Errorf(ctx, "error checking if machine %q is controller: %v", machineID, err)
	} else if isController {
		jobs = append(jobs, model.JobManageModel)
	}
	status.Jobs = jobs

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
	machineUUID, err := c.machineService.GetMachineUUID(ctx, coremachine.Name(machineID))
	if err != nil {
		logger.Debugf(ctx, "error retrieving uuid for machine: %q, %w", machineID, err)
	} else {
		instid, displayName, err = c.machineService.GetInstanceIDAndName(ctx, machineUUID)
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
		for _, mAddr := range c.ipAddresses[coremachine.Name(machineID)] {
			switch mAddr.Scope {
			case network.ScopeMachineLocal, network.ScopeLinkLocal:
				continue
			}
			status.IPAddresses = append(status.IPAddresses, mAddr.AddressValue)
		}
		if len(status.IPAddresses) == 0 {
			logger.Debugf(ctx, "no IP addresses fetched for machine %q", instid)
			// At least give it the newly created DNSName address, if it exists.
			if addr.Value != "" {
				status.IPAddresses = append(status.IPAddresses, addr.Value)
			}
		}

		linkLayerDevices := c.linkLayerDevices[coremachine.Name(machineID)]
		status.NetworkInterfaces = transform.SliceToMap(linkLayerDevices, func(llDev domainnetwork.NetInterface) (string, params.NetworkInterface) {
			spaces := set.NewStrings()
			for _, addr := range llDev.Addrs {
				spaces.Add(addr.Space)
			}
			return llDev.Name, params.NetworkInterface{
				IPAddresses:    transform.Slice(llDev.Addrs, func(net domainnetwork.NetAddr) string { return net.AddressValue }),
				MACAddress:     unptr(llDev.MACAddress),
				Gateway:        unptr(llDev.GatewayAddress),
				DNSNameservers: llDev.DNSAddresses,
				Space:          strings.Join(spaces.Values(), " "),
				IsUp:           llDev.IsEnabled}
		})
		logger.Tracef(ctx, "NetworkInterfaces: %+v", status.NetworkInterfaces)
	} else {
		status.InstanceId = "pending"
	}

	constraints := c.machineConstraints.Machine(machineID)
	status.Constraints = constraints.String()

	hc, err := c.machineService.GetHardwareCharacteristics(ctx, machineUUID)
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
	charmProfiles, err := c.machineService.AppliedLXDProfileNames(ctx, machineUUID)
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

func (c *statusContext) processRelations(ctx context.Context) []params.RelationStatus {
	var out []params.RelationStatus
	for _, current := range c.relationsByID {
		var eps []params.EndpointStatus
		var scope charm.RelationScope
		var relationInterface string
		for _, ep := range current.Endpoints {
			eps = append(eps, params.EndpointStatus{
				ApplicationName: ep.ApplicationName,
				Name:            ep.Name,
				Role:            string(ep.Role),
				Subordinate:     c.isSubordinate(&ep),
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

func (c *statusContext) isSubordinate(ep *relation.Endpoint) bool {
	application, ok := c.allAppsUnitsCharmBindings.applications[ep.ApplicationName]
	if !ok {
		return false
	}
	return isSubordinate(ep, application)
}

func isSubordinate(ep *relation.Endpoint, application statusservice.Application) bool {
	return ep.Scope == charm.ScopeContainer && application.Subordinate
}

func (c *statusContext) processApplications(ctx context.Context) map[string]params.ApplicationStatus {
	applicationsMap := make(map[string]params.ApplicationStatus)
	for name, app := range c.allAppsUnitsCharmBindings.applications {
		applicationsMap[name] = c.processApplication(ctx, name, app)
	}
	return applicationsMap
}

func (c *statusContext) processApplicationExposedEndpoints(ctx context.Context, name string, application statusservice.Application) (map[string]params.ExposedEndpoint, error) {
	// If the application is not exposed, then we don't need to try and get the
	// exposed endpoints for the application. This reduces the number of default
	// calls to the application service.
	if !application.Exposed {
		return nil, nil
	}

	exposedEndpoints, err := c.applicationService.GetExposedEndpoints(ctx, name)
	if err != nil {
		return nil, err
	}
	return c.mapExposedEndpointsFromDomain(exposedEndpoints)
}

func (c *statusContext) processApplication(ctx context.Context, name string, application statusservice.Application) params.ApplicationStatus {
	exposedEndpoints, err := c.processApplicationExposedEndpoints(ctx, name, application)
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

	if latestCharm, ok := c.allAppsUnitsCharmBindings.latestCharms[application.CharmLocator.WithoutRevision()]; ok && !latestCharm.IsZero() {
		processedStatus.CanUpgradeTo, err = charms.CharmURLFromLocator(latestCharm.Name, latestCharm)
		if err != nil {
			return params.ApplicationStatus{Err: apiservererrors.ServerError(err)}
		}
	}

	processedStatus.Relations, processedStatus.SubordinateTo, err = c.processApplicationRelations(name, application)
	if err != nil {
		processedStatus.Err = apiservererrors.ServerError(err)
		return processedStatus
	}
	units := application.Units
	if !application.Subordinate {
		processedStatus.Units = c.processUnits(ctx, units, charmURL)
	}

	if application.WorkloadVersion != nil {
		processedStatus.WorkloadVersion = *application.WorkloadVersion
	}

	processedStatus.EndpointBindings = transform.Map(
		c.allAppsUnitsCharmBindings.endpointBindings[name],
		func(k string, v network.SpaceName) (string, string) { return k, v.String() },
	)

	// IAAS applications have all the information they need in the application
	// status. CAAS applications have some additional information.
	if c.model.Type == model.IAAS {
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

func (c *statusContext) mapExposedEndpointsFromDomain(
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
				sp := c.spaceInfos.GetByID(network.SpaceUUID(spaceID))
				if sp == nil {
					return nil, internalerrors.Errorf("space with ID %q: %w", spaceID, errors.NotFound)
				}

				spaceNames[i] = sp.Name.String()
			}
			mappedParam.ExposeToSpaces = spaceNames
		}

		res[endpointName] = mappedParam
	}

	return res, nil
}

type offerStatus struct {
	crossmodel.ApplicationOffer
	err                  error
	charmURL             string
	activeConnectedCount int
	totalConnectedCount  int
}

func (c *statusContext) processOffers() map[string]params.ApplicationOfferStatus {
	offers := make(map[string]params.ApplicationOfferStatus)
	for name, offer := range c.offers {
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

func (c *statusContext) processUnits(ctx context.Context, units map[coreunit.Name]statusservice.Unit, applicationCharm string) map[string]params.UnitStatus {
	unitsMap := make(map[string]params.UnitStatus)
	for name, unit := range units {
		unitsMap[name.String()] = c.processUnit(ctx, name, unit, applicationCharm)
	}
	return unitsMap
}

func (c *statusContext) unitMachineID(unit statusservice.Unit) coremachine.Name {
	if !unit.Subordinate {
		// machineID will be empty if not currently assigned.
		var machineName coremachine.Name
		if unit.MachineName != nil {
			machineName = *unit.MachineName
		}
		return machineName
	}

	// We're a subordinate, so we need to look at the principal unit. If it's
	// not set, we can't do anything, so return early.
	if unit.PrincipalName == nil {
		return ""
	}

	// Locate the principal unit.
	if unit, ok := c.unitByName(*unit.PrincipalName); ok {
		return c.unitMachineID(unit)
	}
	return ""
}

func (c *statusContext) unitPublicAddress(unit statusservice.Unit) string {
	machine := c.allMachines[c.unitMachineID(unit).String()]
	if machine == nil {
		return ""
	}
	// We don't care if the machine doesn't have an address yet.
	addr, _ := machine.PublicAddress()
	return addr.Value
}

func (c *statusContext) processUnit(ctx context.Context, unitName coreunit.Name, unit statusservice.Unit, applicationCharm string) params.UnitStatus {
	var result params.UnitStatus
	if prs, ok := c.allOpenPortRanges[unitName]; ok {
		result.OpenedPorts = transform.Slice(prs, network.PortRange.String)
	}
	if c.model.Type == model.IAAS {
		result.PublicAddress = c.unitPublicAddress(unit)
	} else if unit.K8sProviderID != nil {
		// For CAAS units we want to provide the container address.
		// TODO (stickupkid): Get the K8s pod address once link layer devices
		// are available.
		result.ProviderId = *unit.K8sProviderID
	}
	if !unit.Subordinate && unit.MachineName != nil {
		result.Machine = unit.MachineName.String()
	}

	if unitCharm, err := charms.CharmURLFromLocator(unit.CharmLocator.Name, unit.CharmLocator); err == nil && unitCharm != applicationCharm {
		result.Charm = unitCharm
	}
	if unit.WorkloadVersion != nil {
		result.WorkloadVersion = *unit.WorkloadVersion
	}
	result.AgentStatus, result.WorkloadStatus = c.processUnitAndAgentStatus(unit, unitName)

	if leader := c.leaders[unit.ApplicationName]; leader == unitName.String() {
		result.Leader = true
	}

	subUnits := unit.SubordinateNames
	if len(subUnits) == 0 {
		return result
	}

	// Handle any subordinate units.
	result.Subordinates = make(map[string]params.UnitStatus)
	for _, name := range subUnits {
		subUnit, ok := c.unitByName(name)
		if !ok {
			continue
		}

		subCharmURL, ok := c.allAppsUnitsCharmBindings.applicationCharmURL[subUnit.ApplicationName]
		if !ok {
			logger.Debugf(ctx, "missing subordinate application %q", subUnit.ApplicationName)
			continue
		}

		result.Subordinates[name.String()] = c.processUnit(ctx, name, subUnit, subCharmURL)
	}

	return result
}

func (c *statusContext) unitByName(name coreunit.Name) (statusservice.Unit, bool) {
	applicationName := name.Application()
	application, ok := c.allAppsUnitsCharmBindings.applications[applicationName]
	if !ok {
		return statusservice.Unit{}, false
	}
	unit, ok := application.Units[name]
	return unit, ok
}

func (c *statusContext) processApplicationRelations(
	name string,
	application statusservice.Application,
) (related map[string][]string, subord []string, err error) {
	subordSet := make(set.Strings)
	related = make(map[string][]string)
	relations := c.relations[name]
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

func (c *statusContext) unitToMachine(ctx context.Context, unitTag names.UnitTag) (names.MachineTag, error) {
	unit, ok := c.unitByName(coreunit.Name(unitTag.Id()))
	if !ok || unit.MachineName == nil {
		return names.MachineTag{}, internalerrors.Errorf("unit %v: %w", unitTag, errors.NotFound)
	}
	return names.NewMachineTag(string(*unit.MachineName)), nil
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
func (c *statusContext) processUnitAndAgentStatus(unit statusservice.Unit, unitName coreunit.Name) (params.DetailedStatus, params.DetailedStatus) {
	detailedAgentStatus := params.DetailedStatus{
		Status:  unit.AgentStatus.Status.String(),
		Info:    unit.AgentStatus.Message,
		Data:    filterStatusData(unit.AgentStatus.Data),
		Since:   unit.AgentStatus.Since,
		Life:    unit.Life,
		Version: unit.AgentVersion,
	}
	detailedWorkloadStatus := params.DetailedStatus{
		Status: unit.WorkloadStatus.Status.String(),
		Info:   unit.WorkloadStatus.Message,
		Data:   filterStatusData(unit.WorkloadStatus.Data),
		Since:  unit.WorkloadStatus.Since,
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
func (c *statusContext) processMachine(ctx context.Context, m *state.Machine) (out params.DetailedStatus) {
	machineName := coremachine.Name(m.Id())
	statusInfo, err := c.statusService.GetMachineStatus(ctx, machineName)
	if internalerrors.Is(err, machineerrors.MachineNotFound) {
		err = internalerrors.Errorf("machine %q not found", machineName).Add(errors.NotFound)
	}
	populateStatusFromStatusInfoAndErr(&out, statusInfo, err)

	out.Life = processLife(m)

	if t, err := m.AgentTools(); err == nil {
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

// unptr dereferences a pointer of type T and returns its value.
// Returns the zero value of T if the pointer is nil.
func unptr[T any](ptr *T) (v T) {
	if ptr == nil {
		return
	}
	return *ptr
}
