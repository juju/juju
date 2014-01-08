// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmversionupdater

import (
	"fmt"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/charm"
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
	context := versionContext{}
	var err error
	if context.services, context.units, err = fetchAllServicesAndUnits(api.state); err != nil {
		return params.ErrorResult{common.ServerError(err)}, nil
	}
	context.gatherServices()
	context.processRevisionInformation()

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

	// repoRevisions holds the charm revisions found on the charm store or local repo.
	repoRevisions map[string]charmRevision
	// serviceRevisions holds the charm revisions for the deployed services.
	serviceRevisions map[string]serviceRevision
}

func (context *versionContext) gatherServices() {
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
		context.processUnits(service.Name())
	}
}

func (context *versionContext) processUnits(serviceName string) {
	units := context.units[serviceName]
	for _, unit := range units {
		url, ok := unit.CharmURL()
		if ok {
			context.serviceRevisions[serviceName].unitVersions[unit.Name()] = charmRevision{revision: url.Revision}
		}
	}
}

func (context *versionContext) processRevisionInformation() {
	// Look up the revision information for all the deployed charms.
	context.retrieveRevisionInformation()

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
			if unitVersion.revision <= 0 && serviceVersion.revision > 0 {
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

func (context *versionContext) retrieveRevisionInformation() {
	// We have previously recorded all the charms in use by the deployed services.
	// Now, look up their latest versions from the relevant repos and record that.
	// First organise the charms into the repo from whence they came.
	repoCharms := make(map[charm.Repository][]*charm.URL)
	for _, charmRevisionInfo := range context.repoRevisions {
		curl := charmRevisionInfo.curl
		repo, err := charm.InferRepository(curl, "")
		if err != nil {
			// We'll get an error for local repos since we don't yet
			// support passing in a path to the local repo. This may
			// change if the need arises but would require an extra
			// parameter to the status command.
			continue
		}
		repoCharms[repo] = append(repoCharms[repo], curl)
	}

	// For each repo, do a bulk call to get the revision info
	// for all the charms from that repo.
	for repo, curls := range repoCharms {
		infos, err := repo.Info(curls...)
		if err != nil {
			// We won't let a problem finding the revision info kill
			// the entire status command.
			logger.Errorf("finding charm revision info: %v", err)
			break
		}
		// Record the results.
		for i, info := range infos {
			curl := curls[i]
			baseURL := curl.WithRevision(-1).String()
			charmRevisionInfo := context.repoRevisions[baseURL]
			if len(info.Errors) > 0 {
				// Just report the first error if there are issues.
				charmRevisionInfo.err = fmt.Errorf("%v", info.Errors[0])
				context.repoRevisions[baseURL] = charmRevisionInfo
				continue
			}
			charmRevisionInfo.revision = info.Revision
			context.repoRevisions[baseURL] = charmRevisionInfo
		}
	}
}
