// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/errors"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	corecharm "github.com/juju/juju/core/charm"
	coreseries "github.com/juju/juju/core/series"
)

// CharmHubClient represents the methods required of a
// client to install or upgrade a CharmHub charm.
type CharmHubClient interface {
	DownloadAndRead(ctx context.Context, resourceURL *url.URL, archivePath string, options ...charmhub.DownloadOption) (*charm.CharmArchive, error)
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

type chRepo struct {
	client CharmHubClient
}

// ResolveWithPreferredChannel defines a way using the given charm URL and
// charm origin (platform and channel) to locate a matching charm against the
// Charmhub API.
//
// There are a few things to note in the attempt to resolve the charm and it's
// supporting series.
//
//    1. The algorithm for this is terrible. For charmstore lookups, only one
//       request is required, unfortunately for Charmhub the worst case for this
//       will be 2.
//       Most of the initial requests from the client will hit this first time
//       around (think `juju deploy foo`) without a series (client can then
//       determine what to call the real request with) will be default of 2
//       requests.
//    2. Attempting to find the default series will require 2 requests so that
//       we can find the correct charm ID ensure that the default series exists
//       along with the revision.
//    3. In theory we could just return most of this information without the
//       re-request, but we end up with missing data and potential incorrect
//       charm downloads later.
//
// When charmstore goes, we could potentially rework how the client requests
// the store.
func (c *chRepo) ResolveWithPreferredChannel(curl *charm.URL, origin corecharm.Origin) (*charm.URL, corecharm.Origin, []string, error) {
	logger.Tracef("Resolving CharmHub charm %q with origin %v", curl, origin)

	if curl.Revision != -1 {
		return nil, corecharm.Origin{}, nil, errors.Errorf("specifying a revision is not supported, please use a channel.")
	}

	input := origin

	// First attempt to find the charm based on the only input provided.
	res, err := c.refreshOne(curl, input)
	if err != nil {
		return nil, corecharm.Origin{}, nil, errors.Annotatef(err, "resolving with preferred channel")
	}

	// resolvableBases holds a slice of supported bases from the subsequent
	// refresh API call. The bases can inform the consumer of the API about what
	// they can also install *IF* the retry resolution uses a base that doesn't
	// match their requirements. This can happen in the client if the series
	// selection also wants to consider model-config default-series after the
	// call.
	var resolvableBases []corecharm.Platform
	if res.Error != nil {
		retryResult, err := c.retryResolveWithPreferredChannel(curl, input, res.Error)
		if err != nil {
			return nil, corecharm.Origin{}, nil, errors.Annotatef(err, "resolving with preferred channel")
		}

		res = retryResult.refreshResponse
		resolvableBases = retryResult.bases
		input = retryResult.origin

		// Fill these back on the origin, so that we can fix the issue of
		// bundles passing back "all" on the response type.
		// Note: we can be sure these have at least one, because of the
		// validation logic in retry method.
		origin.Platform.OS = resolvableBases[0].OS
		origin.Platform.Series = resolvableBases[0].Series
	}

	// Use the channel that was actually picked by the API. This should
	// account for the closed tracks in a given channel.
	channel, err := charm.ParseChannelNormalize(res.EffectiveChannel)
	if err != nil {
		return nil, corecharm.Origin{}, nil, errors.Annotatef(err, "invalid channel")
	}

	// Ensure we send the updated curl back, with all the correct segments.
	revision := res.Entity.Revision
	resCurl := curl.
		WithSeries(input.Platform.Series).
		WithArchitecture(input.Platform.Architecture).
		WithRevision(revision)

	// Create a resolved origin.  Keep the original values for ID and Hash, if
	// any were passed in.  ResolveWithPreferredChannel is called for both
	// charms to be deployed, and charms which are being upgraded.
	// Only charms being upgraded will have an ID and Hash. Those values should
	// only ever be updated in FindDownloadURL.
	resOrigin := corecharm.Origin{
		Source:   origin.Source,
		ID:       origin.ID,
		Hash:     origin.Hash,
		Type:     string(res.Entity.Type),
		Channel:  &channel,
		Revision: &revision,
		Platform: input.Platform,
	}

	outputOrigin, err := sanitizeCharmOrigin(resOrigin, origin)
	if err != nil {
		return nil, corecharm.Origin{}, nil, errors.Trace(err)
	}
	logger.Tracef("Resolved CharmHub charm %q with origin %v", resCurl, outputOrigin)

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
	supportedSeries := []string{
		outputOrigin.Platform.Series,
	}
	if len(resolvableBases) > 0 {
		supportedSeries = make([]string, len(resolvableBases))
		for k, base := range resolvableBases {
			supportedSeries[k] = base.Series
		}
	}

	return resCurl, outputOrigin, supportedSeries, nil
}

type retryResolveResult struct {
	refreshResponse transport.RefreshResponse
	origin          corecharm.Origin
	bases           []corecharm.Platform
}

// retryResolveWithPreferredChannel will attempt to inspect the transport
// APIError and deterimine if a retry is possible with the information gathered
// from the error.
func (c *chRepo) retryResolveWithPreferredChannel(curl *charm.URL, origin corecharm.Origin, resErr *transport.APIError) (*retryResolveResult, error) {
	var (
		err   error
		bases []corecharm.Platform
	)
	switch resErr.Code {
	case transport.ErrorCodeInvalidCharmPlatform, transport.ErrorCodeInvalidCharmBase:
		logger.Tracef("Invalid charm platform %q %v - Default Base: %v", curl, origin, resErr.Extra.DefaultBases)

		if bases, err = c.selectNextBase(resErr.Extra.DefaultBases, origin); err != nil {
			return nil, errors.Annotatef(err, "selecting next bases")
		}

	case transport.ErrorCodeRevisionNotFound:
		logger.Tracef("Revision not found %q %v - Releases: %v", curl, origin, resErr.Extra.Releases)

		if bases, err = c.selectNextReleases(resErr.Extra.Releases, origin); err != nil {
			return nil, errors.Annotatef(err, "selecting releases")
		}

	default:
		return nil, errors.Errorf("resolving error: %s", resErr.Message)
	}

	if len(bases) == 0 {
		return nil, errors.Wrap(resErr, errors.Errorf("no channels available for selection"))
	}
	base := bases[0]

	p := origin.Platform
	p.OS = base.OS
	p.Series = base.Series

	origin.Platform = p

	if origin.Platform.Series == "" {
		return nil, errors.NotValidf("series for %s", curl.Name)
	}

	logger.Tracef("Refresh again with %q %v", curl, origin)
	res, err := c.refreshOne(curl, origin)
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

// DownloadCharm downloads the provided download URL from CharmHub using the
// provided archive path.
// A charm archive is returned.
func (c *chRepo) DownloadCharm(resourceURL string, archivePath string) (*charm.CharmArchive, error) {
	logger.Tracef("DownloadCharm from CharmHub %q", resourceURL)
	curl, err := url.Parse(resourceURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.client.DownloadAndRead(context.TODO(), curl, archivePath)
}

// FindDownloadURL returns the url from which to download the CharmHub
// charm defined by the provided curl and charm origin.  An updated
// charm origin is also returned with the ID and hash for the charm
// to be downloaded.  If the provided charm origin has no ID, it is
// assumed that the charm is being installed, not refreshed.
func (c *chRepo) FindDownloadURL(curl *charm.URL, origin corecharm.Origin) (*url.URL, corecharm.Origin, error) {
	logger.Tracef("FindDownloadURL %v %v", curl, origin)

	refreshRes, err := c.refreshOne(curl, origin)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	if refreshRes.Error != nil {
		return nil, corecharm.Origin{}, errors.Errorf("%s: %s", refreshRes.Error.Code, refreshRes.Error.Message)
	}

	resOrigin := origin
	// We've called Refresh with the install action.  Now update the
	// charm ID and Hash values saved.  This is the only place where
	// they should be saved.
	resOrigin.ID = refreshRes.Entity.ID
	resOrigin.Hash = refreshRes.Entity.Download.HashSHA256

	durl, err := url.Parse(refreshRes.Entity.Download.URL)
	if err != nil {
		return nil, corecharm.Origin{}, errors.Trace(err)
	}
	outputOrigin, err := sanitizeCharmOrigin(resOrigin, origin)
	return durl, outputOrigin, errors.Trace(err)
}

// ListResources returns the resources for a given charm and origin.
func (c *chRepo) ListResources(curl *charm.URL, origin corecharm.Origin) ([]charmresource.Resource, error) {
	logger.Tracef("CharmHub ListResources %q", curl)

	resCurl, resOrigin, _, err := c.ResolveWithPreferredChannel(curl, origin)
	if isErrSelection(err) {
		var channel string
		if origin.Channel != nil {
			channel = origin.Channel.String()
		}
		return nil, errors.Errorf("unable to locate charm %q with matching channel %q", curl.Name, channel)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

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

func (c *chRepo) refreshOne(curl *charm.URL, origin corecharm.Origin) (transport.RefreshResponse, error) {
	cfg, err := refreshConfig(curl, origin)
	if err != nil {
		return transport.RefreshResponse{}, errors.Trace(err)
	}
	logger.Tracef("Locate charm using: %v", cfg)
	result, err := c.client.Refresh(context.TODO(), cfg)
	if err != nil {
		return transport.RefreshResponse{}, errors.Trace(err)
	}
	if len(result) != 1 {
		return transport.RefreshResponse{}, errors.Errorf("more than 1 result found")
	}

	return result[0], nil
}

func (c *chRepo) selectNextBase(bases []transport.Base, origin corecharm.Origin) ([]corecharm.Platform, error) {
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
	results := make([]corecharm.Platform, len(compatible))
	for k, base := range compatible {
		track, err := channelTrack(base.Channel)
		if err != nil {
			return nil, errors.Annotate(err, "base")
		}
		series, err := coreseries.VersionSeries(track)
		if err != nil {
			return nil, errors.Trace(err)
		}

		results[k] = corecharm.Platform{
			Architecture: base.Architecture,
			OS:           base.Name,
			Series:       series,
		}

	}

	return results, nil
}

func (c *chRepo) selectNextReleases(releases []transport.Release, origin corecharm.Origin) ([]corecharm.Platform, error) {
	if len(releases) == 0 {
		return nil, errors.Errorf("no releases available")
	}
	if origin.Platform.Series == "" {
		// If the origin is empty, then we want to help the user out
		// by display a series of suggestions to try.
		suggestions := composeSuggestions(releases, origin)
		var s string
		if len(suggestions) > 0 {
			s = fmt.Sprintf("; suggestions: %v", strings.Join(suggestions, ", "))
		}
		return nil, errSelection{
			err: errors.Errorf("no charm or bundle matching channel or platform%s", s),
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
	// MethodRevision utilizes requesting by the revision only.
	MethodRevision Method = "revision"
	// MethodChannel utilizes requesting by the channel only.
	MethodChannel Method = "channel"
	// MethodID utilizes requesting by the id and channel (falls back to
	// latest/stable if channel is found).
	MethodID Method = "id"
)

// refreshConfig creates a RefreshConfig for the given input.
// If the origin.ID is not set, a install refresh config is returned. For
//   install. Channel and Revision are mutually exclusive in the api, only
//   one will be used.  Channel first, Revision is a fallback.
// If the origin.ID is set, a refresh config is returned.
func refreshConfig(curl *charm.URL, origin corecharm.Origin) (charmhub.RefreshConfig, error) {
	// Work out the correct install method.
	var rev int
	var method Method
	if origin.Revision != nil && *origin.Revision >= 0 {
		rev = *origin.Revision
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

	if origin.ID == "" && channel != "" {
		method = MethodChannel
	}
	// Bundles can not use method IDs, which in turn forces a refresh.
	if method == MethodRevision && !transport.BundleType.Matches(origin.Type) && origin.ID != "" {
		method = MethodID
	}

	// origin.Platform.Series could be a series or a version. In reality it will
	// be a series (focal, groovy), but to be on the safe side we should
	// validate and fallback if it really isn't a version.
	// The refresh will fail if it's wrong with a revision not found, which
	// will be fine for now.
	track, _ := channelTrack(origin.Platform.Series)
	baseChannel, err := coreseries.VersionSeries(track)
	if err != nil {
		baseChannel = origin.Platform.Series
	}

	var (
		cfg charmhub.RefreshConfig

		base = charmhub.RefreshBase{
			Architecture: origin.Platform.Architecture,
			Name:         origin.Platform.OS,
			Channel:      baseChannel,
		}
	)
	switch method {
	case MethodChannel:
		// Install from just the name and the channel. If there is no origin ID,
		// we haven't downloaded this charm before.
		// Try channel first.
		cfg, err = charmhub.InstallOneFromChannel(curl.Name, channel, base)
	case MethodRevision:
		// If there is a revision, install it using that. If there is no origin
		// ID, we haven't downloaded this charm before.
		// No channel, try with revision.
		cfg, err = charmhub.InstallOneFromRevision(curl.Name, rev, base)
	case MethodID:
		// This must be a charm upgrade if we have an ID.  Use the refresh
		// action for metric keeping on the CharmHub side.
		cfg, err = charmhub.RefreshOne(origin.ID, rev, channel, base)
	default:
		return nil, errors.NotValidf("origin %v", origin)
	}
	return cfg, err
}

func composeSuggestions(releases []transport.Release, origin corecharm.Origin) []string {
	channelSeries := make(map[string][]string)
	for _, release := range releases {
		base := release.Base
		arch := base.Architecture
		track, err := channelTrack(base.Channel)
		if err != nil {
			logger.Errorf("invalid base channel %v: %s", base.Channel, err)
			continue
		}
		series, err := coreseries.VersionSeries(track)
		if err != nil {
			logger.Errorf("converting version to series: %s", err)
			continue
		}
		if arch == "all" {
			arch = origin.Platform.Architecture
		}
		if arch != origin.Platform.Architecture {
			continue
		}
		if series == "all" {
			series = origin.Platform.Series
		}
		channelSeries[release.Channel] = append(channelSeries[release.Channel], series)
	}

	var suggestions []string
	// Sort for latest channels to be suggested first.
	// Assumes that releases have normalized channels.
	for _, r := range charm.Risks {
		risk := string(r)
		if values, ok := channelSeries[risk]; ok {
			suggestions = append(suggestions, fmt.Sprintf("%s with %s", risk, strings.Join(values, ", ")))
			delete(channelSeries, risk)
		}
	}

	for channel, values := range channelSeries {
		suggestions = append(suggestions, fmt.Sprintf("%s with %s", channel, strings.Join(values, ", ")))
	}
	return suggestions
}

func selectReleaseByArchAndChannel(releases []transport.Release, origin corecharm.Origin) ([]corecharm.Platform, error) {
	var (
		empty   = origin.Channel == nil
		channel charm.Channel
	)
	if !empty {
		channel = origin.Channel.Normalize()
	}
	var results []corecharm.Platform
	for _, release := range releases {
		base := release.Base

		arch, os := base.Architecture, base.Name
		track, err := channelTrack(base.Channel)
		if err != nil {
			return nil, errors.Trace(err)
		}
		series, err := coreseries.VersionSeries(track)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if (empty || channel.String() == release.Channel) && (arch == "all" || arch == origin.Platform.Architecture) {
			results = append(results, corecharm.Platform{
				Architecture: origin.Platform.Architecture,
				OS:           os,
				Series:       series,
			})
		}
	}
	return results, nil
}

func channelTrack(channel string) (string, error) {
	// Base channel can be found as either just the version `20.04` (focal)
	// or as `20.04/latest` (focal latest). We should future proof ourself
	// for now and just drop the risk on the floor.
	ch, err := charm.ParseChannel(channel)
	if err != nil {
		return "", errors.Trace(err)
	}
	if ch.Track == "" {
		return "", errors.NotValidf("channel track")
	}
	return ch.Track, nil
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
