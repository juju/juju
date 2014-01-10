// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmversionupdater

import (
	"fmt"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

var logger = loggo.GetLogger("juju.state.apiserver.keymanager")

// CharmVersionUpdater defines the methods on the charmversionupdater API end point.
type CharmVersionUpdater interface {
	UpdateVersions() (params.ErrorResult, error)
}

// CharmVersionUpdaterAPI implements the CharmVersionUpdater interface and is the concrete
// implementation of the api end point.
type CharmVersionUpdaterAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

var _ CharmVersionUpdater = (*CharmVersionUpdaterAPI)(nil)

// NewCharmVersionUpdaterAPI creates a new server-side charmversionupdater API end point.
func NewCharmVersionUpdaterAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*CharmVersionUpdaterAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthStateManager() {
		return nil, common.ErrPerm
	}
	return &CharmVersionUpdaterAPI{
		state: st, resources: resources, authorizer: authorizer}, nil
}

// UpdateVersions checks the revision information from the charm store for all deployed charms
// and updates each service and unit revision status to indicate whether the deployed charm
// is out of date or not.
func (api *CharmVersionUpdaterAPI) UpdateVersions() (params.ErrorResult, error) {
	// First get the uuid for the environment to use when querying the charm store.
	env, err := api.state.Environment()
	if err != nil {
		return params.ErrorResult{common.ServerError(err)}, nil
	}
	uuid := env.UUID()

	context := versionContext{}
	if context.services, context.units, err = fetchAllServicesAndUnits(api.state); err != nil {
		return params.ErrorResult{common.ServerError(err)}, nil
	}
	// Gather charm and revision info for deployed services and units.
	context.collateDeployedServiceRevisions()
	// Look up the revision information for all the deployed charms.
	if err = context.retrieveRevisionInformation(uuid); err != nil {
		return params.ErrorResult{common.ServerError(err)}, nil
	}
	// Update the revision status for services and units according to the
	// latest available charm revisions.
	context.updateServiceUnitRevisionStatus()

	return params.ErrorResult{}, nil
}

// fetchAllServicesAndUnits returns a map from service name to service
// and a map from service name to unit name to unit.
func fetchAllServicesAndUnits(st *state.State) (map[string]*state.Service, map[string]map[string]*state.Unit, error) {
	svcMap := make(map[string]*state.Service)
	unitMap := make(map[string]map[string]*state.Unit)
	services, err := st.AllServices()
	if err != nil {
		return nil, nil, err
	}
	for _, s := range services {
		svcMap[s.Name()] = s
		units, err := s.AllUnits()
		if err != nil {
			return nil, nil, err
		}
		if len(units) == 0 {
			continue
		}
		svcUnitMap := make(map[string]*state.Unit)
		for _, u := range units {
			svcUnitMap[u.Name()] = u
		}
		unitMap[s.Name()] = svcUnitMap
	}
	return svcMap, unitMap, nil
}

// charmRevision is used to hold the revision number for a charm and any error occurring
// when attempting to find out the revision.
type charmRevision struct {
	curl     *charm.URL
	revision int
	err      error
}

// serviceRevision is used to hold the revision number for a service and its principal units.
type serviceRevision struct {
	charmRevision
	unitVersions map[string]charmRevision
}

// versionContext is used to hold the current state when updating charm version information.
type versionContext struct {
	// a map from service name to service
	services map[string]*state.Service
	// a map from service name to unit name to unit
	units map[string]map[string]*state.Unit

	// repoRevisions holds the charm revisions found on the charm store.
	// Any charms which come from a local repository are ignored.
	repoRevisions map[string]charmRevision
	// serviceRevisions holds the charm revisions for the deployed services.
	serviceRevisions map[string]serviceRevision
}

func (context *versionContext) collateDeployedServiceRevisions() {
	context.repoRevisions = make(map[string]charmRevision)
	context.serviceRevisions = make(map[string]serviceRevision)

	for _, s := range context.services {
		context.processService(s)
	}
}

func (context *versionContext) processService(service *state.Service) {
	url, _ := service.CharmURL()

	// Record the basic charm information so it can be bulk processed later to
	// get the available revision numbers from the repo.
	baseCharm := url.WithRevision(-1)

	context.serviceRevisions[service.Name()] = serviceRevision{
		charmRevision: charmRevision{curl: baseCharm, revision: url.Revision},
		unitVersions:  make(map[string]charmRevision),
	}
	context.repoRevisions[baseCharm.String()] = charmRevision{curl: baseCharm}

	if service.IsPrincipal() {
		context.collateDeployedUnitRevisions(service.Name())
	}
}

func (context *versionContext) collateDeployedUnitRevisions(serviceName string) {
	units := context.units[serviceName]
	for _, unit := range units {
		url, ok := unit.CharmURL()
		if ok {
			context.serviceRevisions[serviceName].unitVersions[unit.Name()] = charmRevision{revision: url.Revision}
		}
	}
}

func (context *versionContext) updateServiceUnitRevisionStatus() {
	// For each service, compare the latest charm version with what the service has
	// and record the status line.
	for serviceName, s := range context.services {
		serviceStatus := ""
		serviceVersion := context.serviceRevisions[serviceName]
		repoCharmRevision := context.repoRevisions[serviceVersion.curl.String()]
		if repoCharmRevision.err != nil {
			serviceStatus = fmt.Sprintf("unknown: %v", repoCharmRevision.err)
			goto setServiceRevision
		}
		// Should never happen but in case a charm is not found on the charm store we ignore it.
		if repoCharmRevision.revision == 0 {
			goto setServiceRevision
		}
		if serviceVersion.err != nil {
			serviceStatus = fmt.Sprintf("unknown: %v", serviceVersion.err)
			goto setServiceRevision
		}
		// Only report if service revision is out of date.
		if repoCharmRevision.revision > serviceVersion.revision {
			serviceStatus = fmt.Sprintf("out of date (available: %d)", repoCharmRevision.revision)
		}
	setServiceRevision:
		if err := s.SetRevisionStatus(serviceStatus); err != nil {
			logger.Errorf("cannot update revision status for service %s: %v", serviceName, err)
		}

		// And now the units for the service.
		for unitName, u := range context.units[serviceName] {
			unitStatus := ""
			unitVersion := serviceVersion.unitVersions[unitName]
			if unitVersion.revision <= 0 && serviceVersion.revision > 0 && repoCharmRevision.revision > 0 {
				unitStatus = "unknown"
				goto setUnitRevision
			}
			// Only report if unit revision is known, and is different to service revision, and is out of date.
			if unitVersion.revision != serviceVersion.revision && repoCharmRevision.revision > unitVersion.revision {
				unitStatus = fmt.Sprintf("out of date (available: %d)", repoCharmRevision.revision)
			}
		setUnitRevision:
			if err := u.SetRevisionStatus(unitStatus); err != nil {
				logger.Errorf("cannot update revision status for unit %s: %v", unitName, err)
			}
		}
	}
}

func (context *versionContext) retrieveRevisionInformation(uuid string) error {
	// We have previously recorded all the charms in use by the deployed services.
	// Now, look up their latest versions from the charm store and record that so that
	// we may then compare what's in the store with what's deployed.
	var curls []*charm.URL
	for _, charmRevisionInfo := range context.repoRevisions {
		curl := charmRevisionInfo.curl
		if curl.Schema == "local" {
			// Version checking for charms from local repositories is not
			// currently supported, since we don't yet support passing in
			// a path to the local repo. This may change if the need arises.
			continue
		}
		curls = append(curls, curl)
	}

	// Do a bulk call to get the revision info for all charms.
	logger.Infof("retrieving revision information for %d charms", len(curls))
	store := charm.Store.WithJujuAttrs("environment_uuid=" + uuid)
	revInfo, err := store.Latest(curls...)
	if err != nil {
		return log.LoggedErrorf(logger, "finding charm revision info: %v", err)
	}
	// Record the results.
	for i, info := range revInfo {
		curl := curls[i]
		baseURL := curl.WithRevision(-1).String()
		charmRevisionInfo := context.repoRevisions[baseURL]
		if info.Err != nil {
			charmRevisionInfo.err = info.Err
			context.repoRevisions[baseURL] = charmRevisionInfo
			continue
		}
		charmRevisionInfo.revision = info.Revision
		context.repoRevisions[baseURL] = charmRevisionInfo
	}
	return nil
}
