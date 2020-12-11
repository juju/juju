// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"strconv"
	"strings"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/charm/v8/resource"
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
	state State

	newCharmstoreClient newCharmstoreClientFunc
	newCharmhubClient   newCharmhubClientFunc
}

type newCharmstoreClientFunc func(st State) (charmstore.Client, error)
type newCharmhubClientFunc func(st State, metadata map[string]string) (CharmhubRefreshClient, error)

var _ CharmRevisionUpdater = (*CharmRevisionUpdaterAPI)(nil)

// NewCharmRevisionUpdaterAPI creates a new server-side charmrevisionupdater API end point.
func NewCharmRevisionUpdaterAPI(ctx facade.Context) (*CharmRevisionUpdaterAPI, error) {
	if !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	newCharmstoreClient := func(st State) (charmstore.Client, error) {
		controllerCfg, err := st.ControllerConfig()
		if err != nil {
			return charmstore.Client{}, errors.Trace(err)
		}
		return charmstore.NewCachingClient(state.MacaroonCache{MacaroonCacheState: st}, controllerCfg.CharmStoreURL())
	}
	newCharmhubClient := func(st State, metadata map[string]string) (CharmhubRefreshClient, error) {
		return common.CharmhubClient(charmhubClientStateShim{state: st}, logger, metadata)
	}
	return NewCharmRevisionUpdaterAPIState(
		StateShim{State: ctx.State()},
		newCharmstoreClient,
		newCharmhubClient,
	)
}

// NewCharmRevisionUpdaterAPIState creates a new charmrevisionupdater API
// with a State interface directly (mainly for use in tests).
func NewCharmRevisionUpdaterAPIState(
	state State,
	newCharmstoreClient newCharmstoreClientFunc,
	newCharmhubClient newCharmhubClientFunc,
) (*CharmRevisionUpdaterAPI, error) {
	return &CharmRevisionUpdaterAPI{
		state:               state,
		newCharmstoreClient: newCharmstoreClient,
		newCharmhubClient:   newCharmhubClient,
	}, nil
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
	latest, err := api.retrieveLatestCharmInfo()
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
		err := resources.SetCharmStoreResources(info.appID, info.resources, info.timestamp)
		if err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

type latestCharmInfo struct {
	url       *charm.URL
	timestamp time.Time
	revision  int
	resources []resource.Resource
	appID     string
}

type appInfo struct {
	id       string
	charmURL *charm.URL
}

// retrieveLatestCharmInfo looks up the charm store to return the charm URLs for the
// latest revision of the deployed charms.
func (api *CharmRevisionUpdaterAPI) retrieveLatestCharmInfo() ([]latestCharmInfo, error) {
	applications, err := api.state.AllApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Partition the charms into charmhub vs charmstore so we can query each
	// batch separately.
	var (
		charmstoreIDs  []charmstore.CharmID
		charmstoreApps []appInfo
		charmhubIDs    []charmhubID
		charmhubApps   []appInfo
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
			charmhubApps = append(charmhubApps, appInfo{
				id:       application.ApplicationTag().Id(),
				charmURL: curl,
			})

		case charm.CharmStore.Matches(curl.Schema):
			origin := application.CharmOrigin()
			if origin == nil {
				// If this fails, we have big problems, so make this Errorf
				logger.Errorf("charm %s has no origin, skipping", curl)
				continue
			}
			cid := charmstore.CharmID{
				URL:     curl,
				Channel: application.Channel(),
				Metadata: map[string]string{
					"series": origin.Platform.Series,
					"arch":   origin.Platform.Architecture,
				},
			}
			charmstoreIDs = append(charmstoreIDs, cid)
			charmstoreApps = append(charmstoreApps, appInfo{
				id:       application.ApplicationTag().Id(),
				charmURL: curl,
			})

		default:
			return nil, errors.NotValidf("charm schema %q", curl.Schema)
		}
	}

	var latest []latestCharmInfo
	if len(charmstoreIDs) > 0 {
		storeLatest, err := api.fetchCharmstoreInfos(charmstoreIDs, charmstoreApps)
		if err != nil {
			return nil, errors.Trace(err)
		}
		latest = append(latest, storeLatest...)
	}
	if len(charmhubIDs) > 0 {
		hubLatest, err := api.fetchCharmhubInfos(charmhubIDs, charmhubApps)
		if err != nil {
			return nil, errors.Trace(err)
		}
		latest = append(latest, hubLatest...)
	}

	return latest, nil
}

func (api *CharmRevisionUpdaterAPI) fetchCharmstoreInfos(ids []charmstore.CharmID, appInfos []appInfo) ([]latestCharmInfo, error) {
	client, err := api.newCharmstoreClient(api.state)
	if err != nil {
		return nil, errors.Trace(err)
	}
	metadata, err := apiMetadata(api.state)
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := api.state.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	metadata["environment_uuid"] = model.UUID() // duplicates model_uuid, but send to charmstore for legacy reasons
	results, err := charmstore.LatestCharmInfo(client, ids, metadata)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var latest []latestCharmInfo
	for i, result := range results {
		if i >= len(appInfos) {
			logger.Errorf("retrieved more results (%d) than charmstore applications (%d)",
				i, len(appInfos))
			break
		}
		if result.Error != nil {
			logger.Errorf("retrieving charm info for %s: %v", ids[i].URL, result.Error)
			continue
		}
		appInfo := appInfos[i]
		latest = append(latest, latestCharmInfo{
			url:       result.CharmInfo.OriginalURL.WithRevision(result.CharmInfo.LatestRevision),
			timestamp: result.CharmInfo.Timestamp,
			revision:  result.CharmInfo.LatestRevision,
			resources: result.CharmInfo.LatestResources,
			appID:     appInfo.id,
		})
	}
	return latest, nil
}

func (api *CharmRevisionUpdaterAPI) fetchCharmhubInfos(ids []charmhubID, appInfos []appInfo) ([]latestCharmInfo, error) {
	metadata, err := apiMetadata(api.state)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client, err := api.newCharmhubClient(api.state, metadata)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results, err := charmhubLatestCharmInfo(client, ids)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var latest []latestCharmInfo
	for i, result := range results {
		if i >= len(appInfos) {
			logger.Errorf("retrieved more results (%d) than charmhub applications (%d)",
				i, len(appInfos))
			break
		}
		if result.error != nil {
			logger.Errorf("retrieving charm info for ID %s: %v", ids[i].id, result.error)
			continue
		}
		appInfo := appInfos[i]
		latest = append(latest, latestCharmInfo{
			url:       appInfo.charmURL.WithRevision(result.revision),
			timestamp: result.timestamp,
			revision:  result.revision,
			resources: result.resources,
			appID:     appInfo.id,
		})
	}
	return latest, nil
}

// apiMetadata returns a map containing metadata key/value pairs to
// send to the charmhub or charmstore API for tracking metrics.
func apiMetadata(st State) (map[string]string, error) {
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
