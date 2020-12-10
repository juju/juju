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

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	corecharm "github.com/juju/juju/core/charm"
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
	// Look up the information for all the deployed charms. This is the
	// "expensive" part.
	latest, err := retrieveLatestCharmInfo(api.state)
	if err != nil {
		return errors.Trace(err)
	}

	// Process the resulting info for each charm.
	resources, err := api.state.Resources()
	if err != nil {
		return errors.Trace(err)
	}
	for _, info := range latest {
		// First, add a charm placeholder to the model for each.
		if err := api.state.AddCharmPlaceholder(info.url); err != nil {
			return errors.Trace(err)
		}

		// Then save the resources
		tag := info.application.ApplicationTag()
		err := resources.SetCharmStoreResources(tag.Id(), info.resources, info.timestamp)
		if err != nil {
			return errors.Trace(err)
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
	return charmstore.NewCachingClient(state.MacaroonCache{MacaroonCacheState: st}, controllerCfg.CharmStoreURL())
}

// NewCharmhubClient instantiates a new charmhub client (exported for testing).
var NewCharmhubClient = func(st *state.State, metadata map[string]string) (CharmhubRefreshClient, error) {
	return common.CharmhubClient(stateShim{State: st}, logger, metadata)
}

type stateShim struct {
	*state.State
}

func (s stateShim) Model() (common.ConfigModel, error) {
	return s.State.Model()
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
		switch {
		case charm.Local.Matches(curl.Schema):
			continue

		case charm.CharmHub.Matches(curl.Schema):
			origin := application.CharmOrigin()
			if origin == nil {
				// If this fails, we have big problems, so make this Errorf
				logger.Errorf("charm %s has no origin, skipping", curl)
				continue
			}
			if origin.Revision == nil || origin.Channel == nil || origin.Platform == nil {
				logger.Errorf("charm %s has missing revision (%p), channel (%p), or platform (%p), skipping",
					curl, origin.Revision, origin.Channel, origin.Platform)
				continue
			}
			channel, err := corecharm.MakeChannel(origin.Channel.Track, origin.Channel.Risk, origin.Channel.Branch)
			if err != nil {
				return nil, errors.Trace(err)
			}
			cid := charmhubID{
				id:       origin.ID,
				revision: *origin.Revision,
				channel:  channel.String(),
				os:       strings.ToLower(origin.Platform.OS), // charmhub API requires lowercase OS name
				series:   origin.Platform.Series,
				arch:     origin.Platform.Architecture,
			}
			charmhubIDs = append(charmhubIDs, cid)
			charmhubApps = append(charmhubApps, application)

		case charm.CharmStore.Matches(curl.Schema):
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

	var latest []latestCharmInfo
	if len(charmstoreIDs) > 0 {
		storeLatest, err := fetchCharmstoreInfos(st, charmstoreIDs, charmstoreApps)
		if err != nil {
			return nil, errors.Trace(err)
		}
		latest = append(latest, storeLatest...)
	}
	if len(charmhubIDs) > 0 {
		hubLatest, err := fetchCharmhubInfos(st, charmhubIDs, charmhubApps)
		if err != nil {
			return nil, errors.Trace(err)
		}
		latest = append(latest, hubLatest...)
	}

	return latest, nil
}

func fetchCharmstoreInfos(st *state.State, ids []charmstore.CharmID, apps []*state.Application) ([]latestCharmInfo, error) {
	client, err := NewCharmStoreClient(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	metadata, err := apiMetadata(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	metadata["environment_uuid"] = st.ModelUUID() // duplicates model_uuid, but send to charmstore for legacy reasons
	results, err := charmstore.LatestCharmInfo(client, ids, metadata)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var latest []latestCharmInfo
	for i, result := range results {
		if i >= len(apps) {
			logger.Errorf("retrieved more results (%d) than charmstore applications (%d)",
				i, len(apps))
			break
		}
		if result.Error != nil {
			logger.Errorf("retrieving charm info for %s: %v", ids[i].URL, result.Error)
			continue
		}
		application := apps[i]
		latest = append(latest, latestCharmInfo{
			url:         result.CharmInfo.OriginalURL.WithRevision(result.CharmInfo.LatestRevision),
			timestamp:   result.CharmInfo.Timestamp,
			revision:    result.CharmInfo.LatestRevision,
			resources:   result.CharmInfo.LatestResources,
			application: application,
		})
	}
	return latest, nil
}

func fetchCharmhubInfos(st *state.State, ids []charmhubID, apps []*state.Application) ([]latestCharmInfo, error) {
	metadata, err := apiMetadata(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client, err := NewCharmhubClient(st, metadata)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results, err := charmhubLatestCharmInfo(client, ids)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var latest []latestCharmInfo
	for i, result := range results {
		if i >= len(apps) {
			logger.Errorf("retrieved more results (%d) than charmhub applications (%d)",
				i, len(apps))
			break
		}
		if result.error != nil {
			logger.Errorf("retrieving charm info for ID %s: %v", ids[i].id, result.error)
			continue
		}
		application := apps[i]
		url, _ := application.CharmURL()
		latest = append(latest, latestCharmInfo{
			url:         url.WithRevision(result.revision),
			timestamp:   result.timestamp,
			revision:    result.revision,
			resources:   result.resources,
			application: application,
		})
	}
	return latest, nil
}

// apiMetadata returns a map containing metadata key/value pairs to
// send to the charmhub or charmstore API for tracking metrics.
func apiMetadata(st *state.State) (map[string]string, error) {
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	metadata := map[string]string{
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
