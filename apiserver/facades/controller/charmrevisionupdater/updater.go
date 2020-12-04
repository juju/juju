// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"strconv"
	"strings"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/charm/v8/resource"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/v2/series"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.apiserver.charmrevisionupdater")

// CharmRevisionUpdater defines the methods on the charmrevisionupdater API end point.
type CharmRevisionUpdater interface {
	UpdateLatestRevisions() (params.ErrorResult, error)
}

// CharmRevisionUpdaterAPI implements the CharmRevisionUpdater interface and is the concrete
// implementation of the api end point.
type CharmRevisionUpdaterAPI struct {
	state      *state.State
	resources  facade.Resources
	authorizer facade.Authorizer
}

var _ CharmRevisionUpdater = (*CharmRevisionUpdaterAPI)(nil)

// NewCharmRevisionUpdaterAPI creates a new server-side charmrevisionupdater API end point.
func NewCharmRevisionUpdaterAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*CharmRevisionUpdaterAPI, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &CharmRevisionUpdaterAPI{
		state: st, resources: resources, authorizer: authorizer}, nil
}

// UpdateLatestRevisions retrieves the latest revision information from the charm store for all deployed charms
// and records this information in state.
func (api *CharmRevisionUpdaterAPI) UpdateLatestRevisions() (params.ErrorResult, error) {
	if err := api.updateLatestRevisions(); err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}

func (api *CharmRevisionUpdaterAPI) updateLatestRevisions() error {
	// Get the handlers to use.
	handlers, err := createHandlers(api.state)
	if err != nil {
		return err
	}

	// Look up the information for all the deployed charms. This is the
	// "expensive" part.
	latest, err := retrieveLatestCharmInfo(api.state)
	if err != nil {
		return err
	}

	// Process the resulting info for each charm.
	for _, info := range latest {
		// First, add a charm placeholder to the model for each.
		if err := api.state.AddCharmPlaceholder(info.url); err != nil {
			return err
		}

		// Then run through the handlers.
		tag := info.application.ApplicationTag()
		for _, handler := range handlers {
			if err := handler.HandleLatest(tag, info.resources, info.timestamp); err != nil {
				return err
			}
		}
	}

	return nil
}

// NewCharmStoreClient instantiates a new charm store repository.  Exported so
// we can change it during testing.
var NewCharmStoreClient = func(st *state.State) (charmstore.Client, error) {
	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return charmstore.Client{}, errors.Trace(err)
	}
	return charmstore.NewCachingClient(state.MacaroonCache{State: st}, controllerCfg.CharmStoreURL())
}

type latestCharmInfo struct {
	url         *charm.URL
	timestamp   time.Time
	revision    int
	resources   []resource.Resource
	application *state.Application
}

// retrieveLatestCharmInfo looks up the charm store to return the charm URLs for the
// latest revision of the deployed charms.
func retrieveLatestCharmInfo(st *state.State) ([]latestCharmInfo, error) {
	logger.Infof("TODO retrieveLatestCharmInfo")
	applications, err := st.AllApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Partition the charms into charmhub vs charmstore so we can query each
	// batch separately.
	var (
		charmstoreIDs  []charmstore.CharmID
		charmstoreApps []*state.Application
		charmhubIDs    []charmhubID
		charmhubApps   []*state.Application
	)
	for _, application := range applications {
		curl, _ := application.CharmURL()
		switch curl.Schema {
		case "local":
			continue

		case "ch":
			origin := application.CharmOrigin()
			if origin == nil || origin.Revision == nil {
				logger.Debugf("charm %s has no revision, skipping", curl)
				continue
			}
			os, err := series.GetOSFromSeries(application.Series())
			if err != nil {
				return nil, errors.Trace(err)
			}
			archs, err := deployedArchs(application)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if len(archs) == 0 {
				logger.Debugf("charm %s not deployed to any architectures, skipping", curl)
				continue
			}
			cid := charmhubID{
				id:       origin.ID,
				revision: *origin.Revision,
				channel:  string(application.Channel()),
				os:       strings.ToLower(os.String()), // charmhub API requires lowercase OS name
				series:   application.Series(),
				arch:     archs[0],
			}
			charmhubIDs = append(charmhubIDs, cid)
			charmhubApps = append(charmhubApps, application)

		case "cs":
			archs, err := deployedArchs(application)
			if err != nil {
				return nil, errors.Trace(err)
			}
			cid := charmstore.CharmID{
				URL:     curl,
				Channel: application.Channel(),
				Metadata: map[string]string{
					"series": application.Series(),
					"arch":   strings.Join(archs, ","),
				},
			}
			charmstoreIDs = append(charmstoreIDs, cid)
			charmstoreApps = append(charmstoreApps, application)

		default:
			return nil, errors.NotValidf("charm schema %q", curl.Schema)
		}
	}

	// Fetch info for any charmstore charms.
	var latest []latestCharmInfo
	if len(charmstoreIDs) > 0 {
		client, err := NewCharmStoreClient(st)
		if err != nil {
			return nil, errors.Trace(err)
		}
		metadata, err := charmstoreAPIMetadata(st)
		if err != nil {
			return nil, errors.Trace(err)
		}
		results, err := charmstore.LatestCharmInfo(client, charmstoreIDs, metadata)
		logger.Infof("TODO LatestCharmInfo results=%#v, error=%v", results, err)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for i, result := range results {
			if i >= len(charmstoreApps) {
				logger.Errorf("retrieved more results (%d) than charmstore applications (%d)",
					i, len(charmstoreApps))
				break
			}
			if result.Error != nil {
				logger.Errorf("retrieving charm info for %s: %v", charmstoreIDs[i].URL, result.Error)
				continue
			}
			application := charmstoreApps[i]
			latest = append(latest, latestCharmInfo{
				url:         result.CharmInfo.OriginalURL.WithRevision(result.CharmInfo.LatestRevision),
				timestamp:   result.CharmInfo.Timestamp,
				revision:    result.CharmInfo.LatestRevision,
				resources:   result.CharmInfo.LatestResources,
				application: application,
			})
		}
	}

	// Fetch info for any charmhub charms.
	if len(charmhubIDs) > 0 {
		client, err := common.CharmhubClient(st)
		if err != nil {
			return nil, errors.Trace(err)
		}
		results, err := charmhubLatestCharmInfo(client, charmhubIDs)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for i, result := range results {
			if i >= len(charmhubApps) {
				logger.Errorf("retrieved more results (%d) than charmhub applications (%d)",
					i, len(charmhubApps))
				break
			}
			if result.error != nil {
				logger.Errorf("retrieving charm info for ID %s: %v", charmhubIDs[i].id, result.error)
				continue
			}
			application := charmhubApps[i]
			latest = append(latest, latestCharmInfo{
				url: &charm.URL{ // TODO(benhoyt) - not entirely sure about this?
					Schema:   "ch",
					Name:     result.name,
					Revision: result.revision,
				},
				timestamp:   result.timestamp,
				revision:    result.revision,
				resources:   result.resources,
				application: application,
			})
		}
	}

	return latest, nil
}

// charmstoreAPIMetadata returns a map containing metadata key/value pairs to
// send to the charmstore API for tracking metrics.
func charmstoreAPIMetadata(st *state.State) (map[string]string, error) {
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	metadata := map[string]string{
		"environment_uuid":   model.UUID(),
		"model_uuid":         model.UUID(),
		"controller_uuid":    st.ControllerUUID(),
		"controller_version": version.Current.String(),
		"cloud":              model.CloudName(),
		"cloud_region":       model.CloudRegion(),
		"is_controller":      strconv.FormatBool(model.IsControllerModel()),
	}
	cloud, err := st.Cloud(model.CloudName())
	if err != nil {
		metadata["provider"] = "unknown"
	} else {
		metadata["provider"] = cloud.Type
	}
	return metadata, nil
}

// deployedArchs returns the list of unique architectures this application is
// deployed to, across all the machines it's deployed to.
func deployedArchs(app *state.Application) ([]string, error) {
	machines, err := app.DeployedMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}

	archs := set.NewStrings()
	for _, m := range machines {
		hw, err := m.HardwareCharacteristics()
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return nil, errors.Trace(err)
		}
		arch := hw.Arch
		if arch != nil && *arch != "" {
			archs.Add(*arch)
		}
	}
	return archs.SortedValues(), nil
}
