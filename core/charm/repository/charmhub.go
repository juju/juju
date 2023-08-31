// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/charm/v11"
	charmresource "github.com/juju/charm/v11/resource"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
)

// CharmHubClient describes the API exposed by the charmhub client.
type CharmHubClient interface {
	DownloadAndRead(ctx context.Context, resourceURL *url.URL, archivePath string, options ...charmhub.DownloadOption) (*charm.CharmArchive, error)
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

// CharmHubRepository provides an API for charm-related operations using charmhub.
type CharmHubRepository struct {
	logger Logger
	client CharmHubClient
}

// NewCharmHubRepository returns a new repository instance using the provided
// charmhub client.
func NewCharmHubRepository(logger Logger, chClient CharmHubClient) *CharmHubRepository {
	return &CharmHubRepository{
		logger: logger,
		client: chClient,
	}
}

// ResolveWithPreferredChannel defines a way using the given charm URL and
// charm origin (platform and channel) to locate a matching charm against the
// Charmhub API.
//
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
//
// TODO:
// When charmstore goes, we could potentially rework how the client requests
// the store.
func (c *CharmHubRepository) ResolveWithPreferredChannel(charmURL *charm.URL, argOrigin corecharm.Origin) (*charm.URL, corecharm.Origin, []string, error) {
	c.logger.Tracef("Resolving CharmHub charm %q with origin %v", charmURL, argOrigin)

	requestedOrigin, err := c.validateOrigin(argOrigin)
	if err != nil {
		return nil, corecharm.Origin{}, nil, err
	}

	// First attempt to find the charm based on the only input provided.
	res, err := c.refreshOne(charmURL, requestedOrigin)
	if err != nil {
		return nil, corecharm.Origin{}, nil, errors.Annotatef(err, "resolving with preferred channel")
	}

	// resolvableBases holds a slice of supported bases from the subsequent
	// refresh API call. The bases can inform the consumer of the API about what
	// they can also install *IF* the retry resolution uses a base that doesn't
	// match their requirements. This can happen in the client if the series
	// selection also wants to consider model-config default-base after the
	// call.
	var (
		effectiveChannel  string
		resolvableBases   []corecharm.Platform
		chSuggestedOrigin = requestedOrigin
	)
	switch {
	case res.Error != nil:
		retryResult, err := c.retryResolveWithPreferredChannel(charmURL, requestedOrigin, res.Error)
		if err != nil {
			return nil, corecharm.Origin{}, nil, errors.Trace(err)
		}

		res = retryResult.refreshResponse
		resolvableBases = retryResult.bases
		chSuggestedOrigin = retryResult.origin

		// Fill these back on the origin, so that we can fix the issue of
		// bundles passing back "all" on the response type.
		// Note: we can be sure these have at least one, because of the
		// validation logic in retry method.
		requestedOrigin.Platform.OS = resolvableBases[0].OS
		requestedOrigin.Platform.Channel = resolvableBases[0].Channel

		effectiveChannel = res.EffectiveChannel
	case requestedOrigin.Revision != nil && *requestedOrigin.Revision != -1:
		if len(res.Entity.Bases) > 0 {
			for _, v := range res.Entity.Bases {
				resolvableBases = append(resolvableBases, corecharm.Platform{
					Architecture: v.Architecture,
					OS:           v.Name,
					Channel:      v.Channel,
				})
			}
		}
		// Entities installed by revision do not have an effective channel in the data.
		// A charm with revision X can be in multiple different channels.  The user
		// specified channel for future refresh calls, is located in the origin.
		if res.Entity.Type == transport.CharmType && requestedOrigin.Channel != nil {
			effectiveChannel = requestedOrigin.Channel.String()
		} else if res.Entity.Type == transport.BundleType {
			// This is a hack. A bundle does not require a channel moving forward as refresh
			// by bundle is not implemented, however the following code expects a channel.
			effectiveChannel = "stable"
		}
	default:
		effectiveChannel = res.EffectiveChannel
	}

	// Use the channel that was actually picked by the API. This should
	// account for the closed tracks in a given channel.
	channel, err := charm.ParseChannelNormalize(effectiveChannel)
	if err != nil {
		return nil, corecharm.Origin{}, nil, errors.Annotatef(err, "invalid channel for %q", charmURL)
	}

	// Ensure we send the updated charmURL back, with all the correct segments.
	revision := res.Entity.Revision
	resCurl := charmURL.
		WithArchitecture(chSuggestedOrigin.Platform.Architecture).
		WithRevision(revision)
	// TODO(wallyworld) - does charm url still need a series?
	if chSuggestedOrigin.Platform.Channel != "" {
		series, err := corebase.GetSeriesFromChannel(chSuggestedOrigin.Platform.OS, chSuggestedOrigin.Platform.Channel)
		if err != nil {
			return nil, corecharm.Origin{}, nil, errors.Trace(err)
		}
		resCurl = resCurl.WithSeries(series)
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
		Type:        string(res.Entity.Type),
		Channel:     &channel,
		Revision:    &revision,
		Platform:    chSuggestedOrigin.Platform,
		InstanceKey: requestedOrigin.InstanceKey,
	}

	outputOrigin, err := sanitizeCharmOrigin(resOrigin, requestedOrigin)
	if err != nil {
		return nil, corecharm.Origin{}, nil, errors.Trace(err)
	}
	c.logger.Tracef("Resolved CharmHub charm %q with origin %v", resCurl, outputOrigin)

	// If the callee of the API defines a series and that series is pick and
	// identified as being selected (think `juju deploy --series`) then we will
	// never have to retry. The API will never give us back any other supported
	// series, so we can just pass back what the callee requested.
	// This is the happy path for resolving a charm.
	//
	// Unfortunately, most deployments will not pass a series flag, so we will
	// have to ask the API to give us back a potential base. The supported
	// bases can be passed back as a slice of supported series. The callee can
	// then determine which base they want to use and deploy that accordingly,
	// without another API request.
	// TODO(juju3) - we should use supported channels not series
	var series string
	if outputOrigin.Platform.Channel != "" {
		series, err = corebase.GetSeriesFromChannel(outputOrigin.Platform.OS, outputOrigin.Platform.Channel)
		if err != nil {
			return nil, corecharm.Origin{}, nil, errors.Trace(err)
		}
	}
	supportedSeries := []string{
		series,
	}
	if len(resolvableBases) > 0 {
		supportedSeries = make([]string, len(resolvableBases))
		for k, base := range resolvableBases {
			series, err = corebase.GetSeriesFromChannel(base.OS, base.Channel)
			if err != nil {
				return nil, corecharm.Origin{}, nil, errors.Trace(err)
			}
			supportedSeries[k] = series
		}
	}

	return resCurl, outputOrigin, supportedSeries, nil
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
func (c *CharmHubRepository) retryResolveWithPreferredChannel(charmURL *charm.URL, origin corecharm.Origin, resErr *transport.APIError) (*retryResolveResult, error) {
	var (
		err   error
		bases []corecharm.Platform
	)
	switch resErr.Code {
	case transport.ErrorCodeInvalidCharmPlatform, transport.ErrorCodeInvalidCharmBase:
		c.logger.Tracef("Invalid charm platform %q %v - Default Base: %v", charmURL, origin, resErr.Extra.DefaultBases)

		if bases, err = c.selectNextBases(resErr.Extra.DefaultBases, origin); err != nil {
			return nil, errors.Annotatef(err, "selecting next bases")
		}

	case transport.ErrorCodeRevisionNotFound:
		c.logger.Tracef("Revision not found %q %v - Releases: %v", charmURL, origin, resErr.Extra.Releases)

		if bases, err = c.selectNextBasesFromReleases(resErr.Extra.Releases, origin); err != nil {
			return nil, errors.Annotatef(err, "selecting releases")
		}

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
		return nil, errors.NotValidf("channel for %s", charmURL.Name)
	}

	c.logger.Tracef("Refresh again with %q %v", charmURL, origin)
	res, err := c.refreshOne(charmURL, origin)
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

// DownloadCharm retrieves specified charm from the store and saves its
// contents to the specified path.
func (c *CharmHubRepository) DownloadCharm(charmURL *charm.URL, requestedOrigin corecharm.Origin, archivePath string) (corecharm.CharmArchive, corecharm.Origin, error) {
	c.logger.Tracef("DownloadCharm %q, origin: %q", charmURL, requestedOrigin)

	// Resolve charm URL to a link to the charm blob and keep track of the
	// actual resolved origin which may be different from the requested one.
	resURL, actualOrigin, err := c.GetDownloadURL(charmURL, requestedOrigin)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	charmArchive, err := c.client.DownloadAndRead(context.TODO(), resURL, archivePath)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}

	return charmArchive, actualOrigin, nil
}

// GetDownloadURL returns the url from which to download the CharmHub charm/bundle
// defined by the provided curl and charm origin.  An updated charm origin is
// also returned with the ID and hash for the charm to be downloaded.  If the
// provided charm origin has no ID, it is assumed that the charm is being
// installed, not refreshed.
func (c *CharmHubRepository) GetDownloadURL(charmURL *charm.URL, requestedOrigin corecharm.Origin) (*url.URL, corecharm.Origin, error) {
	c.logger.Tracef("GetDownloadURL %q, origin: %q", charmURL, requestedOrigin)

	refreshRes, err := c.refreshOne(charmURL, requestedOrigin)
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
	outputOrigin, err := sanitizeCharmOrigin(resOrigin, requestedOrigin)
	return durl, outputOrigin, errors.Trace(err)
}

// ListResources returns the resources for a given charm and origin.
func (c *CharmHubRepository) ListResources(charmURL *charm.URL, origin corecharm.Origin) ([]charmresource.Resource, error) {
	c.logger.Tracef("ListResources %q", charmURL)

	resCurl, resOrigin, _, err := c.ResolveWithPreferredChannel(charmURL, origin)
	if isErrSelection(err) {
		var channel string
		if origin.Channel != nil {
			channel = origin.Channel.String()
		}
		return nil, errors.Errorf("unable to locate charm %q with matching channel %q", charmURL.Name, channel)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	// If a revision is included with an install action, no resources will be
	// returned. Resources are dependent on a channel, a specific revision can
	// be in multiple channels.  refreshOne gives priority to a revision if
	// specified.  ListResources is used by the "charm-resources" cli cmd,
	// therefore specific charm revisions are less important.
	resOrigin.Revision = nil
	resp, err := c.refreshOne(resCurl, resOrigin)
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

// GetEssentialMetadata resolves each provided MetadataRequest and returns back
// a slice with the results. The results include the minimum set of metadata
// that is required for deploying each charm.
func (c *CharmHubRepository) GetEssentialMetadata(reqs ...corecharm.MetadataRequest) ([]corecharm.EssentialMetadata, error) {
	if len(reqs) == 0 {
		return nil, nil
	}

	resolvedOrigins := make([]corecharm.Origin, len(reqs))
	refreshCfgs := make([]charmhub.RefreshConfig, len(reqs))
	for reqIdx, req := range reqs {
		// TODO(achilleasa): We should add support for resolving origin
		// batches and move this outside the loop.
		_, resolvedOrigin, _, err := c.ResolveWithPreferredChannel(req.CharmURL, req.Origin)
		if err != nil {
			return nil, errors.Annotatef(err, "resolving origin for %q", req.CharmURL)
		}

		refreshCfg, err := refreshConfig(req.CharmURL, resolvedOrigin)
		if err != nil {
			return nil, errors.Trace(err)
		}

		resolvedOrigins[reqIdx] = resolvedOrigin
		refreshCfgs[reqIdx] = refreshCfg
	}

	refreshResults, err := c.client.Refresh(context.TODO(), charmhub.RefreshMany(refreshCfgs...))
	if err != nil {
		return nil, errors.Trace(err)
	}

	var metaRes = make([]corecharm.EssentialMetadata, len(reqs))
	for resIdx, refreshResult := range refreshResults {
		if refreshResult.Entity.MetadataYAML == "" {
			return nil, errors.Errorf("charmhub refresh response for %q does not include the contents of metadata.yaml", reqs[resIdx].CharmURL)
		}
		chMeta, err := charm.ReadMeta(strings.NewReader(refreshResult.Entity.MetadataYAML))
		if err != nil {
			return nil, errors.Annotatef(err, "parsing metadata.yaml for %q", reqs[resIdx].CharmURL)
		}

		// It's okay for a charm not to have a config.yaml. But the CharmHub
		// API currently returns "{}\n" for charms that have no config.yaml,
		// so handle that as no config as well.
		var chConfig *charm.Config
		configYAML := refreshResult.Entity.ConfigYAML
		if configYAML == "" || strings.TrimSpace(configYAML) == "{}" {
			chConfig = charm.NewConfig()
		} else {
			chConfig, err = charm.ReadConfig(strings.NewReader(configYAML))
			if err != nil {
				return nil, errors.Annotatef(err, "parsing config.yaml for %q", reqs[resIdx].CharmURL)
			}
		}

		chManifest := new(charm.Manifest)
		for _, base := range refreshResult.Entity.Bases {
			baseCh, err := charm.ParseChannelNormalize(base.Channel)
			if err != nil {
				return nil, errors.Annotatef(err, "parsing base channel for %q", reqs[resIdx].CharmURL)
			}

			chManifest.Bases = append(chManifest.Bases, charm.Base{
				Name:          base.Name,
				Channel:       baseCh,
				Architectures: []string{base.Architecture},
			})
		}

		metaRes[resIdx].ResolvedOrigin = resolvedOrigins[resIdx]
		metaRes[resIdx].Meta = chMeta
		metaRes[resIdx].Config = chConfig
		metaRes[resIdx].Manifest = chManifest
	}

	return metaRes, nil
}

func (c *CharmHubRepository) refreshOne(charmURL *charm.URL, origin corecharm.Origin) (transport.RefreshResponse, error) {
	cfg, err := refreshConfig(charmURL, origin)
	if err != nil {
		return transport.RefreshResponse{}, errors.Trace(err)
	}
	c.logger.Tracef("Locate charm using: %v", cfg)
	result, err := c.client.Refresh(context.TODO(), cfg)
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

func (c *CharmHubRepository) selectNextBasesFromReleases(releases []transport.Release, origin corecharm.Origin) ([]corecharm.Platform, error) {
	if len(releases) == 0 {
		return nil, errors.Errorf("no releases available")
	}
	if origin.Platform.Channel == "" {
		// If the user passed in a branch, but not enough information about the
		// arch and channel, then we can help by giving a better error message.
		if origin.Channel != nil && origin.Channel.Branch != "" {
			return nil, errors.Errorf("ambiguous arch and series with channel %q, specify both arch and series along with channel", origin.Channel.String())
		}
		// If the origin is empty, then we want to help the user out
		// by display a series of suggestions to try.
		suggestions := c.composeSuggestions(releases, origin)
		var s string
		if len(suggestions) > 0 {
			s = fmt.Sprintf("\navailable releases are:\n  %v", strings.Join(suggestions, "\n  "))
		}
		var channelName string
		if origin.Channel != nil {
			channelName = origin.Channel.String()
		}
		return nil, errSelection{
			err: errors.Errorf(
				"charm or bundle not found for channel %q, platform %q%s",
				channelName, origin.Platform.String(), s),
		}
	}

	// From the suggestion list, go look up a release that we can retry.
	return selectReleaseByArchAndChannel(releases, origin)
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
func refreshConfig(charmURL *charm.URL, origin corecharm.Origin) (charmhub.RefreshConfig, error) {
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
		cfg, err = charmhub.InstallOneFromChannel(charmURL.Name, channel, base)
	case MethodRevision:
		// If there is a revision, install it using that. If there is no origin
		// ID, we haven't downloaded this charm before.
		cfg, err = charmhub.InstallOneFromRevision(charmURL.Name, rev)
	case MethodID:
		// This must be a charm upgrade if we have an ID.  Use the refresh
		// action for metric keeping on the CharmHub side.
		cfg, err = charmhub.RefreshOne(origin.InstanceKey, origin.ID, rev, channel, base)
	default:
		return nil, errors.NotValidf("origin %v", origin)
	}
	return cfg, err
}

func (c *CharmHubRepository) composeSuggestions(releases []transport.Release, origin corecharm.Origin) []string {
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
		track, err := corecharm.ChannelTrack(release.Base.Channel)
		if err != nil {
			c.logger.Errorf("invalid base channel %v: %s", release.Base.Channel, err)
			continue
		}
		if track == "all" || release.Base.Name == "all" {
			base, err = corebase.ParseBase(origin.Platform.OS, origin.Platform.Channel)
		} else {
			base, err = corebase.ParseBase(release.Base.Name, release.Base.Channel)
		}
		if err != nil {
			c.logger.Errorf("converting version to base: %s", err)
			continue
		}
		channelSeries[release.Channel] = append(channelSeries[release.Channel], base.DisplayString())
	}

	var suggestions []string
	// Sort for latest channels to be suggested first.
	// Assumes that releases have normalized channels.
	for _, r := range charm.Risks {
		risk := string(r)
		if values, ok := channelSeries[risk]; ok {
			suggestions = append(suggestions, fmt.Sprintf("channel %q: available bases are: %s", risk, strings.Join(values, ", ")))
			delete(channelSeries, risk)
		}
	}

	for channel, values := range channelSeries {
		suggestions = append(suggestions, fmt.Sprintf("channel %q: available bases are: %s", channel, strings.Join(values, ", ")))
	}
	return suggestions
}

func selectReleaseByArchAndChannel(releases []transport.Release, origin corecharm.Origin) ([]corecharm.Platform, error) {
	var (
		empty   = origin.Channel == nil
		arch    = origin.Platform.Architecture
		channel charm.Channel
		results []corecharm.Platform
	)
	if !empty {
		channel = origin.Channel.Normalize()
	}
	seen := set.NewStrings()
	for _, release := range releases {
		base := release.Base

		platform, err := corecharm.ParsePlatform(fmt.Sprintf("%s/%s/%s", arch, base.Name, base.Channel))
		if err != nil {
			return nil, errors.Annotate(err, "base")
		}
		if !seen.Contains(platform.String()) && (empty || channel.String() == release.Channel) && (base.Architecture == "all" || base.Architecture == arch) {
			seen.Add(platform.String())
			results = append(results, platform)
		}
	}
	return results, nil
}

// TODO (stickupkid) - Find a common place for this as it's duplicated from
// apiserver/client/resources
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
