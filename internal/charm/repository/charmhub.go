// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
)

// CharmHubClient describes the API exposed by the charmhub client.
type CharmHubClient interface {
	// Download retrieves the specified charm from the store and saves its
	// contents to the specified path. Read the path to get the contents of the
	// charm.
	Download(ctx context.Context, url *url.URL, path string, options ...charmhub.DownloadOption) (*charmhub.Digest, error)

	// ListResourceRevisions returns a list of resources associated with a given
	// charm.
	ListResourceRevisions(ctx context.Context, charm, resource string) ([]transport.ResourceRevision, error)

	// Refresh retrieves the specified charm from the store and returns the
	// metadata and configuration.
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

// CharmHubRepositoryConfig holds the config options require to construct a
// CharmHubRepository.
type CharmHubRepositoryConfig struct {
	// An HTTP client that is injected when making Charmhub API calls.
	CharmhubHTTPClient charmhub.HTTPClient

	// CharmHubURL is the URL to use for CharmHub API calls.
	CharmhubURL string

	Logger logger.Logger
}

// NewCharmHubRepository returns a new repository instance using the provided
// charmhub client.
func NewCharmHubRepository(cfg CharmHubRepositoryConfig) (*CharmHubRepository, error) {
	chClient, err := charmhub.NewClient(charmhub.Config{
		URL:        cfg.CharmhubURL,
		HTTPClient: cfg.CharmhubHTTPClient,
		Logger:     cfg.Logger.Child("charmhub"),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &CharmHubRepository{
		logger: cfg.Logger.Child("charmhubrepo", logger.CHARMHUB),
		client: chClient,
	}, nil
}

// CharmHubRepository provides an API for charm-related operations using charmhub.
type CharmHubRepository struct {
	logger logger.Logger
	client CharmHubClient
}

// ResolveWithPreferredChannel defines a way using the given charm name and
// charm origin (platform and channel) to locate a matching charm against the
// Charmhub API.
func (c *CharmHubRepository) ResolveWithPreferredChannel(ctx context.Context, charmName string, argOrigin corecharm.Origin) (corecharm.ResolvedData, error) {
	c.logger.Tracef(ctx, "Resolving CharmHub charm %q with origin %+v", charmName, argOrigin)

	requestedOrigin, err := c.validateOrigin(argOrigin)
	if err != nil {
		return corecharm.ResolvedData{}, err
	}
	resultURL, resolvedOrigin, resolvedBases, resp, err := c.resolveWithPreferredChannel(ctx, charmName, requestedOrigin)
	if err != nil {
		return corecharm.ResolvedData{}, errors.Trace(err)
	}

	essMeta, err := EssentialMetadataFromResponse(resultURL.Name, resp)
	if err != nil {
		return corecharm.ResolvedData{}, errors.Trace(err)
	}

	// Get the Hash for the origin. Needed in the case where this
	// charm has been downloaded before.
	// The charmhub ID is missing from the origin, this will be filled in
	// during the deploy process.
	resolvedOrigin.Hash = resp.Entity.Download.HashSHA256

	essMeta.ResolvedOrigin = resolvedOrigin

	// DownloadInfo is required for downloading the charm asynchronously.
	essMeta.DownloadInfo = corecharm.DownloadInfo{
		CharmhubIdentifier: resp.Entity.ID,
		DownloadURL:        resp.Entity.Download.URL,
		DownloadSize:       int64(resp.Entity.Download.Size),
	}

	return corecharm.ResolvedData{
		URL:               resultURL,
		EssentialMetadata: essMeta,
		Origin:            resolvedOrigin,
		Platform:          resolvedBases,
	}, nil
}

// ResolveForDeploy combines ResolveWithPreferredChannel, GetEssentialMetadata
// and best effort for repositoryResources into 1 call for server side charm deployment.
// Reducing the number of required calls to a repository.
func (c *CharmHubRepository) ResolveForDeploy(ctx context.Context, arg corecharm.CharmID) (corecharm.ResolvedDataForDeploy, error) {
	c.logger.Tracef(ctx, "Resolving CharmHub charm %q with origin %+v", arg.URL, arg.Origin)

	resultURL, resolvedOrigin, _, resp, resolveErr := c.resolveWithPreferredChannel(ctx, arg.URL.Name, arg.Origin)
	if resolveErr != nil {
		return corecharm.ResolvedDataForDeploy{}, errors.Trace(resolveErr)
	}

	essMeta, err := EssentialMetadataFromResponse(resultURL.Name, resp)
	if err != nil {
		return corecharm.ResolvedDataForDeploy{}, errors.Trace(err)
	}

	// Get ID and Hash for the origin. Needed in the case where this
	// charm has been downloaded before.
	resolvedOrigin.ID = resp.Entity.ID
	resolvedOrigin.Hash = resp.Entity.Download.HashSHA256

	essMeta.ResolvedOrigin = resolvedOrigin

	// DownloadInfo is required for downloading the charm asynchronously.
	essMeta.DownloadInfo = corecharm.DownloadInfo{
		CharmhubIdentifier: resp.Entity.ID,
		DownloadURL:        resp.Entity.Download.URL,
		DownloadSize:       int64(resp.Entity.Download.Size),
	}

	// Resources are best attempt here. If we were able to resolve the charm
	// via a channel, the resource data will be here. If using a revision,
	// then not. However, that does not mean that the charm has no resources.
	resourceResults, err := transformResourceRevision(resp.Entity.Resources)
	if err != nil {
		return corecharm.ResolvedDataForDeploy{}, errors.Trace(err)
	}

	return corecharm.ResolvedDataForDeploy{
		URL:               resultURL,
		EssentialMetadata: essMeta,
		Resources:         resourceResults,
	}, nil
}

// There are a few things to note in the attempt to resolve the charm and it's
// supporting series.
//
//  1. The algorithm for this is terrible. For Charmhub the worst case for this
//     will be 2.
//     Most of the initial requests from the client will hit this first time
//     around (think `juju deploy foo`) without a series (client can then
//     determine what to call the real request with) will be default of 2
//     requests.
//  2. Attempting to find the default series will require 2 requests so that
//     we can find the correct charm ID ensure that the default series exists
//     along with the revision.
//  3. In theory we could just return most of this information without the
//     re-request, but we end up with missing data and potential incorrect
//     charm downloads later.
func (c *CharmHubRepository) resolveWithPreferredChannel(ctx context.Context, charmName string, requestedOrigin corecharm.Origin) (*charm.URL, corecharm.Origin, []corecharm.Platform, transport.RefreshResponse, error) {
	c.logger.Tracef(ctx, "Resolving CharmHub charm %q with origin %v", charmName, requestedOrigin)

	// First attempt to find the charm based on the only input provided.
	response, err := c.refreshOne(ctx, charmName, requestedOrigin)
	if err != nil {
		return nil, corecharm.Origin{}, nil, transport.RefreshResponse{}, errors.Annotatef(err, "resolving with preferred channel")
	}

	// resolvedBases holds a slice of supported bases from the subsequent
	// refresh API call. The bases can inform the consumer of the API about what
	// they can also install *IF* the retry resolution uses a base that doesn't
	// match their requirements. This can happen in the client if the series
	// selection also wants to consider model-config default-base after the
	// call.
	var (
		effectiveChannel  string
		resolvedBases     []corecharm.Platform
		chSuggestedOrigin = requestedOrigin
	)
	switch {
	case response.Error != nil:
		retryResult, err := c.retryResolveWithPreferredChannel(ctx, charmName, requestedOrigin, response.Error)
		if err != nil {
			return nil, corecharm.Origin{}, nil, transport.RefreshResponse{}, errors.Trace(err)
		}

		response = retryResult.refreshResponse
		resolvedBases = retryResult.bases
		chSuggestedOrigin = retryResult.origin

		// Fill these back on the origin, so that we can fix the issue of
		// bundles passing back "all" on the response type.
		// Note: we can be sure these have at least one, because of the
		// validation logic in retry method.
		requestedOrigin.Platform.OS = resolvedBases[0].OS
		requestedOrigin.Platform.Channel = resolvedBases[0].Channel

		effectiveChannel = response.EffectiveChannel
	case requestedOrigin.Revision != nil && *requestedOrigin.Revision != -1:
		if len(response.Entity.Bases) > 0 {
			for _, v := range response.Entity.Bases {
				resolvedBases = append(resolvedBases, corecharm.Platform{
					Architecture: v.Architecture,
					OS:           v.Name,
					Channel:      v.Channel,
				})
			}
		}
		// Entities installed by revision do not have an effective channel in the data.
		// A charm with revision X can be in multiple different channels.  The user
		// specified channel for future refresh calls, is located in the origin.
		if response.Entity.Type == transport.CharmType && requestedOrigin.Channel != nil {
			effectiveChannel = requestedOrigin.Channel.String()
		} else if response.Entity.Type == transport.BundleType {
			// This is a hack. A bundle does not require a channel moving forward as refresh
			// by bundle is not implemented, however the following code expects a channel.
			effectiveChannel = "stable"
		}
	default:
		effectiveChannel = response.EffectiveChannel
	}

	// Use the channel that was actually picked by the API. This should
	// account for the closed tracks in a given channel.
	channel, err := charm.ParseChannelNormalize(effectiveChannel)
	if err != nil {
		return nil, corecharm.Origin{}, nil, transport.RefreshResponse{}, errors.Annotatef(err, "invalid channel for %q", charmName)
	}

	// Ensure we send the updated charmURL back, with all the correct segments.
	revision := response.Entity.Revision
	resCurl := &charm.URL{
		Schema:       "ch",
		Name:         charmName,
		Revision:     revision,
		Architecture: chSuggestedOrigin.Platform.Architecture,
	}

	// Create a resolved origin.  Keep the original values for ID and Hash, if
	// any were passed in.  ResolveWithPreferredChannel is called for both
	// charms to be deployed, and charms which are being upgraded.
	// Only charms being upgraded will have an ID and Hash. Those values should
	// only ever be updated in DownloadURL.
	resOrigin := corecharm.Origin{
		Source:      requestedOrigin.Source,
		ID:          requestedOrigin.ID,
		Hash:        requestedOrigin.Hash,
		Type:        string(response.Entity.Type),
		Channel:     &channel,
		Revision:    &revision,
		Platform:    chSuggestedOrigin.Platform,
		InstanceKey: requestedOrigin.InstanceKey,
	}

	outputOrigin, err := sanitiseCharmOrigin(resOrigin, requestedOrigin)
	if err != nil {
		return nil, corecharm.Origin{}, nil, transport.RefreshResponse{}, errors.Trace(err)
	}
	c.logger.Tracef(ctx, "Resolved CharmHub charm %q with origin %v", resCurl, outputOrigin)

	// If the callee of the API defines a base and that base is pick and
	// identified as being selected (think `juju deploy --base`) then we will
	// never have to retry. The API will never give us back any other supported
	// base, so we can just pass back what the callee requested.
	// This is the happy path for resolving a charm.
	//
	// Unfortunately, most deployments will not pass a base flag, so we will
	// have to ask the API to give us back a potential base. The supported
	// bases can be passed back. The callee can then determine which base they
	// want to use and deploy that accordingly without another API request.
	if len(resolvedBases) == 0 && outputOrigin.Platform.Channel != "" {
		resolvedBases = []corecharm.Platform{outputOrigin.Platform}
	}
	return resCurl, outputOrigin, resolvedBases, response, nil
}

// validateOrigin, validate the origin and maybe fix as follows:
//
//	Platform must have an architecture.
//	Platform can have both an empty Channel AND os.
//	Platform must have channel if os defined.
//	Platform must have os if channel defined.
func (c *CharmHubRepository) validateOrigin(origin corecharm.Origin) (corecharm.Origin, error) {
	p := origin.Platform

	if p.Architecture == "" {
		return corecharm.Origin{}, errors.BadRequestf("origin.Platform requires an Architecture")
	}

	if p.OS != "" && p.Channel == "" {
		return corecharm.Origin{}, errors.BadRequestf("origin.Platform requires a Channel, if OS set")
	}

	if p.OS == "" && p.Channel != "" {
		return corecharm.Origin{}, errors.BadRequestf("origin.Platform requires an OS, if channel set")
	}
	return origin, nil
}

type retryResolveResult struct {
	refreshResponse transport.RefreshResponse
	origin          corecharm.Origin
	bases           []corecharm.Platform
}

// retryResolveWithPreferredChannel will attempt to inspect the transport
// APIError and determine if a retry is possible with the information gathered
// from the error.
func (c *CharmHubRepository) retryResolveWithPreferredChannel(ctx context.Context, charmName string, origin corecharm.Origin, resErr *transport.APIError) (*retryResolveResult, error) {
	var (
		err   error
		bases []corecharm.Platform
	)
	switch resErr.Code {
	case transport.ErrorCodeInvalidCharmPlatform, transport.ErrorCodeInvalidCharmBase:
		c.logger.Tracef(ctx, "Invalid charm base %q %v - Default Base: %v", charmName, origin, resErr.Extra.DefaultBases)

		if bases, err = c.selectNextBases(resErr.Extra.DefaultBases, origin); err != nil {
			return nil, errors.Annotatef(err, "selecting next bases")
		}

	case transport.ErrorCodeRevisionNotFound:
		c.logger.Tracef(ctx, "Revision not found %q %v - Releases: %v", charmName, origin, resErr.Extra.Releases)

		return nil, errors.Annotatef(c.handleRevisionNotFound(ctx, resErr.Extra.Releases, origin), "selecting releases")

	default:
		return nil, errors.Errorf("resolving error: %s", resErr.Message)
	}

	if len(bases) == 0 {
		ch := origin.Channel.String()
		if ch == "" {
			ch = "stable"
		}
		return nil, errors.Wrap(resErr, errors.Errorf("no releases found for channel %q", ch))
	}
	base := bases[0]

	origin.Platform.OS = base.OS
	origin.Platform.Channel = base.Channel

	if origin.Platform.Channel == "" {
		return nil, errors.NotValidf("channel for %s", charmName)
	}

	c.logger.Tracef(ctx, "Refresh again with %q %v", charmName, origin)
	res, err := c.refreshOne(ctx, charmName, origin)
	if err != nil {
		return nil, errors.Annotatef(err, "retrying")
	}
	if resErr := res.Error; resErr != nil {
		return nil, errors.Errorf("resolving retry error: %s", resErr.Message)
	}
	return &retryResolveResult{
		refreshResponse: res,
		origin:          origin,
		bases:           bases,
	}, nil
}

// Download retrieves a blob from the store and saves its contents to the
// specified path.
//
// To get the contents of the blob, read the path on success.
func (c *CharmHubRepository) Download(ctx context.Context, name string, requestedOrigin corecharm.Origin, path string) (corecharm.Origin, *charmhub.Digest, error) {
	c.logger.Tracef(ctx, "Download %q, origin: %q", name, requestedOrigin)

	// Resolve charm URL to a link to the charm blob and keep track of the
	// actual resolved origin which may be different from the requested one.
	resURL, actualOrigin, err := c.GetDownloadURL(ctx, name, requestedOrigin)
	if err != nil {
		return corecharm.Origin{}, nil, errors.Trace(err)
	}

	// Force the sha256 digest to be calculated on download.
	digest, err := c.client.Download(ctx, resURL, path)
	if err != nil {
		return corecharm.Origin{}, nil, errors.Trace(err)
	}

	// Verify the hash if the requested origin has supplied one.
	if digest.SHA256 != requestedOrigin.Hash {
		return corecharm.Origin{}, nil, errors.Errorf("downloaded charm hash %q does not match expected hash %q", digest.SHA256, requestedOrigin.Hash)
	}

	return actualOrigin, digest, nil
}

// GetDownloadURL returns the url from which to download the CharmHub
// charm/bundle defined by the provided charm name and origin.  An updated charm
// origin is also returned with the ID and hash for the charm to be downloaded.
// If the provided charm origin has no ID, it is assumed that the charm is being
// installed, not refreshed.
func (c *CharmHubRepository) GetDownloadURL(ctx context.Context, charmName string, requestedOrigin corecharm.Origin) (*url.URL, corecharm.Origin, error) {
	c.logger.Tracef(ctx, "GetDownloadURL %q, origin: %q", charmName, requestedOrigin)

	refreshRes, err := c.refreshOne(ctx, charmName, requestedOrigin)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	if refreshRes.Error != nil {
		return nil, corecharm.Origin{}, errors.Errorf("%s: %s", refreshRes.Error.Code, refreshRes.Error.Message)
	}

	resOrigin := requestedOrigin

	// We've called Refresh with the install action.  Now update the
	// charm ID and Hash values saved.  This is the only place where
	// they should be saved.
	resOrigin.ID = refreshRes.Entity.ID
	resOrigin.Hash = refreshRes.Entity.Download.HashSHA256

	durl, err := url.Parse(refreshRes.Entity.Download.URL)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	outputOrigin, err := sanitiseCharmOrigin(resOrigin, requestedOrigin)
	return durl, outputOrigin, errors.Trace(err)
}

// ListResources returns the resources for a given charm and origin.
func (c *CharmHubRepository) ListResources(ctx context.Context, charmName string, origin corecharm.Origin) ([]charmresource.Resource, error) {
	c.logger.Tracef(ctx, "ListResources %q", charmName)

	resolved, err := c.ResolveWithPreferredChannel(ctx, charmName, origin)
	if isErrSelection(err) {
		var channel string
		if origin.Channel != nil {
			channel = origin.Channel.String()
		}
		return nil, errors.Errorf("unable to locate charm %q with matching channel %q", charmName, channel)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	// If a revision is included with an install action, no resources will be
	// returned. Resources are dependent on a channel, a specific revision can
	// be in multiple channels.  refreshOne gives priority to a revision if
	// specified.  ListResources is used by the "charm-resources" cli cmd,
	// therefore specific charm revisions are less important.
	resOrigin := resolved.Origin
	resOrigin.Revision = nil
	resp, err := c.refreshOne(ctx, resolved.URL.Name, resOrigin)
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]charmresource.Resource, len(resp.Entity.Resources))
	for i, resource := range resp.Entity.Resources {
		results[i], err = resourceFromRevision(resource)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return results, nil
}

// TODO 30-Nov-2022
// ResolveResources can be made more efficient, some choices left over from
// integration with charmstore style of working.

// ResolveResources looks at the provided charmhub and backend (already
// downloaded) resources to determine which to use. Provided (uploaded) take
// precedence. If charmhub has a newer resource than the back end, use that.
func (c *CharmHubRepository) ResolveResources(ctx context.Context, resources []charmresource.Resource, id corecharm.CharmID) ([]charmresource.Resource, error) {
	revisionResources, err := c.listResourcesIfRevisions(ctx, resources, id.URL.Name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storeResources, err := c.repositoryResources(ctx, id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storeResourcesMap, err := transformResourceRevision(storeResources)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for k, v := range revisionResources {
		storeResourcesMap[k] = v
	}
	resolved, err := c.resolveResources(ctx, resources, storeResourcesMap, id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return resolved, nil
}

func (c *CharmHubRepository) listResourcesIfRevisions(ctx context.Context, resources []charmresource.Resource, charmName string) (map[string]charmresource.Resource, error) {
	results := make(map[string]charmresource.Resource, 0)
	for _, resource := range resources {
		// If not revision is specified, or the resource has already been
		// uploaded, no need to attempt to find it here.
		if resource.Revision == -1 || resource.Origin == charmresource.OriginUpload {
			continue
		}
		refreshResp, err := c.client.ListResourceRevisions(ctx, charmName, resource.Name)
		if err != nil {
			return nil, errors.Annotatef(err, "refreshing charm %q", charmName)
		}
		if len(refreshResp) == 0 {
			return nil, errors.Errorf("no download refresh responses received")
		}
		for _, res := range refreshResp {
			if res.Revision == resource.Revision {
				results[resource.Name], err = resourceFromRevision(refreshResp[0])
				if err != nil {
					return nil, errors.Trace(err)
				}
			}
		}
	}
	return results, nil
}

// repositoryResources composes, a map of details for each of the charm's
// resources. Those details are those associated with the specific
// charm channel. They include the resource's metadata and revision.
// Found via the CharmHub api. ListResources requires charm resolution,
// this method does not.
func (c *CharmHubRepository) repositoryResources(ctx context.Context, id corecharm.CharmID) ([]transport.ResourceRevision, error) {
	curl := id.URL
	origin := id.Origin
	refBase := charmhub.RefreshBase{
		Architecture: origin.Platform.Architecture,
		Name:         origin.Platform.OS,
		Channel:      origin.Platform.Channel,
	}
	var cfg charmhub.RefreshConfig
	var err error
	switch {
	// Do not get resource data via revision here, it is only provided if explicitly
	// asked for by resource revision.  The purpose here is to find a resource revision
	// in the channel, if one was not provided on the cli.
	case origin.ID != "":
		cfg, err = charmhub.DownloadOneFromChannel(ctx, origin.ID, origin.Channel.String(), refBase)
		if err != nil {
			c.logger.Errorf(ctx, "creating resources config for charm (%q, %q): %s", origin.ID, origin.Channel.String(), err)
			return nil, errors.Annotatef(err, "creating resources config for charm %q", curl.String())
		}
	case origin.ID == "":
		cfg, err = charmhub.DownloadOneFromChannelByName(ctx, curl.Name, origin.Channel.String(), refBase)
		if err != nil {
			c.logger.Errorf(ctx, "creating resources config for charm (%q, %q): %s", curl.Name, origin.Channel.String(), err)
			return nil, errors.Annotatef(err, "creating resources config for charm %q", curl.String())
		}
	}
	refreshResp, err := c.client.Refresh(ctx, cfg)
	if err != nil {
		return nil, errors.Annotatef(err, "refreshing charm %q", curl.String())
	}
	if len(refreshResp) == 0 {
		return nil, errors.Errorf("no download refresh responses received")
	}
	resp := refreshResp[0]
	if resp.Error != nil {
		return nil, errors.Annotatef(errors.New(resp.Error.Message), "listing resources for charm %q", curl.String())
	}
	return resp.Entity.Resources, nil
}

// transformResourceRevision transforms resource revision structs in charmhub format into
// charmresource format for use within juju.
func transformResourceRevision(resources []transport.ResourceRevision) (map[string]charmresource.Resource, error) {
	if len(resources) == 0 {
		return nil, nil
	}
	results := make(map[string]charmresource.Resource, len(resources))
	for _, v := range resources {
		var err error
		results[v.Name], err = resourceFromRevision(v)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return results, nil
}

// resolveResources determines the resource info that should actually
// be stored on the controller. That decision is based on the provided
// resources along with those in the charm backend (if any).
func (c *CharmHubRepository) resolveResources(
	ctx context.Context,
	resources []charmresource.Resource,
	storeResources map[string]charmresource.Resource,
	id corecharm.CharmID,
) ([]charmresource.Resource, error) {
	allResolved := make([]charmresource.Resource, len(resources))
	copy(allResolved, resources)
	for i, res := range resources {
		// Note that incoming "upload" resources take precedence over
		// ones already known to the controller, regardless of their
		// origin.
		if res.Origin != charmresource.OriginStore {
			continue
		}

		resolved, err := c.resolveRepositoryResources(ctx, res, storeResources, id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		allResolved[i] = resolved
	}
	return allResolved, nil
}

// resolveRepositoryResources selects the resource info to use. It decides
// between the provided and latest info based on the revision.
func (c *CharmHubRepository) resolveRepositoryResources(
	ctx context.Context,
	res charmresource.Resource,
	storeResources map[string]charmresource.Resource,
	id corecharm.CharmID,
) (charmresource.Resource, error) {
	storeRes, ok := storeResources[res.Name]
	if !ok {
		// This indicates that AddPendingResources() was called for
		// a resource the charm backend doesn't know about (for the
		// relevant charm revision).
		return res, nil
	}

	if res.Revision < 0 {
		// The caller wants to use the charm backend info.
		return storeRes, nil
	}
	if res.Revision == storeRes.Revision {
		// We don't worry about if they otherwise match. Only the
		// revision is significant here. So we use the info from the
		// charm backend since it is authoritative.
		return storeRes, nil
	}
	if res.Fingerprint.IsZero() {
		// The caller wants resource info from the charm backend, but with
		// a different resource revision than the one associated with
		// the charm in the backend.
		return c.resourceInfo(ctx, id.URL, id.Origin, res.Name, res.Revision)
	}
	// The caller fully-specified a resource with a different resource
	// revision than the one associated with the charm in the backend. So
	// we use the provided info as-is.
	return res, nil
}

func (c *CharmHubRepository) resourceInfo(ctx context.Context, curl *charm.URL, origin corecharm.Origin, name string, revision int) (charmresource.Resource, error) {
	var configs []charmhub.RefreshConfig
	var err error

	// Due to async charm downloading we may not always have a charm ID to
	// use for getting resource info, however it is preferred. A charm name
	// is second best due to anticipation of charms being renamed in the
	// future. The charm url may not change, but the ID will reference the
	// new name.
	if origin.ID != "" {
		configs, err = configsByID(ctx, curl, origin, name, revision)
	} else {
		configs, err = configsByName(ctx, curl, origin, name, revision)
	}
	if err != nil {
		return charmresource.Resource{}, err
	}

	refreshResp, err := c.client.Refresh(ctx, charmhub.RefreshMany(configs...))
	if err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	if len(refreshResp) == 0 {
		return charmresource.Resource{}, errors.Errorf("no download refresh responses received")
	}

	for _, resp := range refreshResp {
		if resp.Error != nil {
			return charmresource.Resource{}, errors.Trace(errors.New(resp.Error.Message))
		}

		for _, entity := range resp.Entity.Resources {
			if entity.Name == name && entity.Revision == revision {
				rfr, err := resourceFromRevision(entity)
				return rfr, err
			}
		}
	}
	return charmresource.Resource{}, errors.NotFoundf("charm resource %q at revision %d", name, revision)
}

// EssentialMetadataFromResponse extracts the essential metadata from the
// provided charmhub refresh response.
func EssentialMetadataFromResponse(charmName string, refreshResult transport.RefreshResponse) (corecharm.EssentialMetadata, error) {
	// We only care about charm metadata.
	if refreshResult.Entity.Type != transport.CharmType {
		return corecharm.EssentialMetadata{}, nil
	}

	entity := refreshResult.Entity

	if entity.MetadataYAML == "" {
		return corecharm.EssentialMetadata{}, errors.NotValidf("charmhub refresh response for %q does not include the contents of metadata.yaml", charmName)
	}
	chMeta, err := charm.ReadMeta(strings.NewReader(entity.MetadataYAML))
	if err != nil {
		return corecharm.EssentialMetadata{}, errors.Annotatef(err, "parsing metadata.yaml for %q", charmName)
	}

	configYAML := entity.ConfigYAML
	var chConfig *charm.Config
	// NOTE: Charmhub returns a "{}\n" when no config.yaml exists for
	// the charm, e.g. postgreql. However, this will fail the charm
	// config validation which happens in ReadConfig. Valid config
	// are nil and "Options: {}"
	if configYAML == "" || strings.TrimSpace(configYAML) == "{}" {
		chConfig = charm.NewConfig()
	} else {
		chConfig, err = charm.ReadConfig(strings.NewReader(configYAML))
		if err != nil {
			return corecharm.EssentialMetadata{}, errors.Annotatef(err, "parsing config.yaml for %q", charmName)
		}
	}

	chManifest := new(charm.Manifest)
	for _, base := range entity.Bases {
		baseCh, err := charm.ParseChannelNormalize(base.Channel)
		if err != nil {
			return corecharm.EssentialMetadata{}, errors.Annotatef(err, "parsing base channel for %q", charmName)
		}

		chManifest.Bases = append(chManifest.Bases, charm.Base{
			Name:          base.Name,
			Channel:       baseCh,
			Architectures: []string{base.Architecture},
		})
	}

	return corecharm.EssentialMetadata{
		Meta:     chMeta,
		Config:   chConfig,
		Manifest: chManifest,
		DownloadInfo: corecharm.DownloadInfo{
			CharmhubIdentifier: entity.ID,
			DownloadURL:        entity.Download.URL,
			DownloadSize:       int64(entity.Download.Size),
		},
	}, nil
}

func configsByID(ctx context.Context, curl *charm.URL, origin corecharm.Origin, name string, revision int) ([]charmhub.RefreshConfig, error) {
	var (
		configs []charmhub.RefreshConfig
		refBase = charmhub.RefreshBase{
			Architecture: origin.Platform.Architecture,
			Name:         origin.Platform.OS,
			Channel:      origin.Platform.Channel,
		}
	)
	// Get all the resources for everything and just find out which one matches.
	// The order is expected to be kept so when the response is looped through
	// we get channel, then revision.
	if sChan := origin.Channel.String(); sChan != "" {
		cfg, err := charmhub.DownloadOneFromChannel(ctx, origin.ID, sChan, refBase)
		if err != nil {
			return configs, errors.Trace(err)
		}
		configs = append(configs, cfg)
	}
	if rev := origin.Revision; rev != nil {
		cfg, err := charmhub.DownloadOneFromRevision(ctx, origin.ID, *rev)
		if err != nil {
			return configs, errors.Trace(err)
		}
		if newCfg, ok := charmhub.AddResource(cfg, name, revision); ok {
			cfg = newCfg
		}
		configs = append(configs, cfg)
	}
	if rev := curl.Revision; rev >= 0 {
		cfg, err := charmhub.DownloadOneFromRevision(ctx, origin.ID, rev)
		if err != nil {
			return configs, errors.Trace(err)
		}
		if newCfg, ok := charmhub.AddResource(cfg, name, revision); ok {
			cfg = newCfg
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

func configsByName(ctx context.Context, curl *charm.URL, origin corecharm.Origin, name string, revision int) ([]charmhub.RefreshConfig, error) {
	charmName := curl.Name
	var configs []charmhub.RefreshConfig
	// Get all the resource for everything and just find out which one matches.
	// The order is expected to be kept so when the response is looped through
	// we get channel, then revision.
	if sChan := origin.Channel.String(); sChan != "" {
		refBase := charmhub.RefreshBase{
			Architecture: origin.Platform.Architecture,
			Name:         origin.Platform.OS,
			Channel:      origin.Platform.Channel,
		}
		cfg, err := charmhub.DownloadOneFromChannelByName(ctx, charmName, sChan, refBase)
		if err != nil {
			return configs, errors.Trace(err)
		}
		configs = append(configs, cfg)
	}
	if rev := origin.Revision; rev != nil {
		cfg, err := charmhub.DownloadOneFromRevisionByName(ctx, charmName, *rev)
		if err != nil {
			return configs, errors.Trace(err)
		}
		if newCfg, ok := charmhub.AddResource(cfg, name, revision); ok {
			cfg = newCfg
		}
		configs = append(configs, cfg)
	}
	if rev := curl.Revision; rev >= 0 {
		cfg, err := charmhub.DownloadOneFromRevisionByName(ctx, charmName, rev)
		if err != nil {
			return configs, errors.Trace(err)
		}
		if newCfg, ok := charmhub.AddResource(cfg, name, revision); ok {
			cfg = newCfg
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

func (c *CharmHubRepository) refreshOne(ctx context.Context, charmName string, origin corecharm.Origin) (transport.RefreshResponse, error) {
	cfg, err := refreshConfig(ctx, charmName, origin)
	if err != nil {
		return transport.RefreshResponse{}, errors.Trace(err)
	}
	c.logger.Tracef(ctx, "Locate charm using: %v", cfg)
	result, err := c.client.Refresh(ctx, cfg)
	if err != nil {
		return transport.RefreshResponse{}, errors.Trace(err)
	}
	if len(result) != 1 {
		return transport.RefreshResponse{}, errors.Errorf("more than 1 result found")
	}

	return result[0], nil
}

func (c *CharmHubRepository) selectNextBases(bases []transport.Base, origin corecharm.Origin) ([]corecharm.Platform, error) {
	if len(bases) == 0 {
		return nil, errors.Errorf("no bases available")
	}
	// We've got a invalid charm platform error, the error should contain
	// a valid platform to query again to get the right information. If
	// the platform is empty, consider it a failure.
	var compatible []transport.Base
	for _, base := range bases {
		if base.Architecture != origin.Platform.Architecture {
			continue
		}
		compatible = append(compatible, base)
	}
	if len(compatible) == 0 {
		return nil, errors.NotFoundf("bases matching architecture %q", origin.Platform.Architecture)
	}

	// Serialize all the platforms into core entities.
	var results []corecharm.Platform
	seen := set.NewStrings()
	for _, base := range compatible {
		platform, err := corecharm.ParsePlatform(fmt.Sprintf("%s/%s/%s", base.Architecture, base.Name, base.Channel))
		if err != nil {
			return nil, errors.Annotate(err, "base")
		}
		if !seen.Contains(platform.String()) {
			seen.Add(platform.String())
			results = append(results, platform)
		}
	}

	return results, nil
}

func (c *CharmHubRepository) handleRevisionNotFound(ctx context.Context, releases []transport.Release, origin corecharm.Origin) error {
	if len(releases) == 0 {
		return errors.Errorf("no releases available")
	}
	// If the user passed in a branch, but not enough information about the
	// arch and channel, then we can help by giving a better error message.
	if origin.Channel != nil && origin.Channel.Branch != "" {
		return errors.Errorf("ambiguous arch and series with channel %q, specify both arch and series along with channel", origin.Channel.String())
	}
	// Help the user out by creating a list of channel/base suggestions to try.
	suggestions := c.composeSuggestions(ctx, releases, origin)
	var s string
	if len(suggestions) > 0 {
		s = fmt.Sprintf("\navailable releases are:\n  %v", strings.Join(suggestions, "\n  "))
	}
	// If the origin's channel is nil, one wasn't specified by the user,
	// so we requested "stable", which indicates the charm's default channel.
	// However, at the time we're writing this message, we do not know what
	// the charm's default channel is.
	var channelString string
	if origin.Channel != nil {
		channelString = fmt.Sprintf("for channel %q", origin.Channel.String())
	} else {
		channelString = "in the charm's default channel"
	}

	return errSelection{
		err: errors.Errorf(
			"charm or bundle not found %s, base %q%s",
			channelString, origin.Platform.String(), s),
	}
}

type errSelection struct {
	err error
}

func (e errSelection) Error() string {
	return e.err.Error()
}

func isErrSelection(err error) bool {
	_, ok := errors.Cause(err).(errSelection)
	return ok
}

// Method describes the method for requesting the charm using the RefreshAPI.
type Method string

const (
	// MethodRevision utilizes an install action by the revision only. A
	// channel must be in the origin, however it's not used in this request,
	// but saved in the origin for future use.
	MethodRevision Method = "revision"
	// MethodChannel utilizes an install action by the channel only.
	MethodChannel Method = "channel"
	// MethodID utilizes an refresh action by the id, revision and
	// channel (falls back to latest/stable if channel is not found).
	MethodID Method = "id"
)

// refreshConfig creates a RefreshConfig for the given input.
// If the origin.ID is not set, a install refresh config is returned. For
// install. Channel and Revision are mutually exclusive in the api, only
// one will be used.
//
// If the origin.ID is set, a refresh config is returned.
//
// NOTE: There is one idiosyncrasy of this method.  The charm URL and and
// origin have a revision number in them when called by GetDownloadURL
// to install a charm. Potentially causing an unexpected install by revision.
// This is okay as all of the data is ready and correct in the origin.
func refreshConfig(ctx context.Context, charmName string, origin corecharm.Origin) (charmhub.RefreshConfig, error) {
	// Work out the correct install method.
	rev := -1
	var method Method
	if origin.Revision != nil && *origin.Revision >= 0 {
		rev = *origin.Revision
	}
	if origin.ID == "" && rev != -1 {
		method = MethodRevision
	}

	var (
		channel         string
		nonEmptyChannel = origin.Channel != nil && !origin.Channel.Empty()
	)

	// Select the appropriate channel based on the supplied origin.
	// We need to ensure that we always, always normalize the incoming channel
	// before we hit the refresh API.
	if nonEmptyChannel {
		channel = origin.Channel.Normalize().String()
	} else if method != MethodRevision {
		channel = corecharm.DefaultChannel.Normalize().String()
	}

	if method != MethodRevision && channel != "" {
		method = MethodChannel
	}

	// Bundles can not use method IDs, which in turn forces a refresh.
	if !transport.BundleType.Matches(origin.Type) && origin.ID != "" {
		method = MethodID
	}

	var (
		cfg charmhub.RefreshConfig
		err error

		base = charmhub.RefreshBase{
			Architecture: origin.Platform.Architecture,
			Name:         origin.Platform.OS,
			Channel:      origin.Platform.Channel,
		}
	)
	switch method {
	case MethodChannel:
		// Install from just the name and the channel. If there is no origin ID,
		// we haven't downloaded this charm before.
		// Try channel first.
		cfg, err = charmhub.InstallOneFromChannel(ctx, charmName, channel, base)
	case MethodRevision:
		// If there is a revision, install it using that. If there is no origin
		// ID, we haven't downloaded this charm before.
		cfg, err = charmhub.InstallOneFromRevision(ctx, charmName, rev)
	case MethodID:
		// This must be a charm upgrade if we have an ID.  Use the refresh
		// action for metric keeping on the CharmHub side.
		cfg, err = charmhub.RefreshOne(ctx, origin.InstanceKey, origin.ID, rev, channel, base)
	default:
		return nil, errors.NotValidf("origin %v", origin)
	}
	return cfg, err
}

func (c *CharmHubRepository) composeSuggestions(ctx context.Context, releases []transport.Release, origin corecharm.Origin) []string {
	charmRisks := set.NewStrings()
	for _, v := range charm.Risks {
		charmRisks.Add(string(v))
	}
	channelSeries := make(map[string][]string)
	for _, release := range releases {
		arch := release.Base.Architecture
		if arch == "all" {
			arch = origin.Platform.Architecture
		}
		if arch != origin.Platform.Architecture {
			continue
		}
		var (
			base corebase.Base
			err  error
		)

		channel, err := corebase.ParseChannel(release.Base.Channel)
		if err != nil {
			c.logger.Errorf(ctx, "invalid base channel %v: %s", release.Base.Channel, err)
			continue
		}
		if channel.Track == "all" || release.Base.Name == "all" {
			base, err = corebase.ParseBase(origin.Platform.OS, origin.Platform.Channel)
		} else {
			base, err = corebase.ParseBase(release.Base.Name, release.Base.Channel)
		}
		if err != nil {
			c.logger.Errorf(ctx, "converting version to base: %s", err)
			continue
		}
		// Now that we have default tracks other than latest:
		// If a channel is risk only, add latest as the track
		// to be more clear for the user facing error message.
		// At this point, we do not know the default channel,
		// or if the charm has one, therefore risk only output
		// is ambiguous.
		charmChannel := release.Channel
		if charmRisks.Contains(charmChannel) {
			charmChannel = "latest/" + charmChannel
		}
		channelSeries[charmChannel] = append(channelSeries[charmChannel], base.DisplayString())
	}

	var suggestions []string
	for channel, values := range channelSeries {
		suggestions = append(suggestions, fmt.Sprintf("channel %q: available bases are: %s", channel, strings.Join(values, ", ")))
	}
	return suggestions
}

func resourceFromRevision(rev transport.ResourceRevision) (charmresource.Resource, error) {
	resType, err := charmresource.ParseType(rev.Type)
	if err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	fp, err := charmresource.ParseFingerprint(rev.Download.HashSHA384)
	if err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	r := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        rev.Name,
			Type:        resType,
			Path:        rev.Filename,
			Description: rev.Description,
		},
		Origin:      charmresource.OriginStore,
		Revision:    rev.Revision,
		Fingerprint: fp,
		Size:        int64(rev.Download.Size),
	}
	return r, nil
}
