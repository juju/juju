// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"archive/zip"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/juju/charm/v12"
	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/base"
	api "github.com/juju/juju/api/client/resources"
	apicharm "github.com/juju/juju/api/common/charm"
	commoncharms "github.com/juju/juju/api/common/charms"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
)

// Client allows access to the charms API end point.
type Client struct {
	base.ClientFacade
	*commoncharms.CharmInfoClient
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the charms API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Charms")
	commonClient := commoncharms.NewCharmInfoClient(backend)
	return &Client{ClientFacade: frontend, CharmInfoClient: commonClient, facade: backend}
}

// CharmToResolve holds the charm url and it's channel to be resolved.
type CharmToResolve struct {
	URL         *charm.URL
	Origin      apicharm.Origin
	SwitchCharm bool
}

// ResolvedCharm holds resolved charm data.
type ResolvedCharm struct {
	URL            *charm.URL
	Origin         apicharm.Origin
	SupportedBases []corebase.Base
	Error          error
}

// ResolveCharms resolves the given charm URLs with an optionally specified
// preferred channel.
func (c *Client) ResolveCharms(charms []CharmToResolve) ([]ResolvedCharm, error) {
	args := params.ResolveCharmsWithChannel{
		Resolve: make([]params.ResolveCharmWithChannel, len(charms)),
	}
	for i, ch := range charms {
		args.Resolve[i] = params.ResolveCharmWithChannel{
			Reference:   ch.URL.String(),
			Origin:      ch.Origin.ParamsCharmOrigin(),
			SwitchCharm: ch.SwitchCharm,
		}
	}
	if c.BestAPIVersion() < 7 {
		var result params.ResolveCharmWithChannelResultsV6
		if err := c.facade.FacadeCall("ResolveCharms", args, &result); err != nil {
			return nil, errors.Trace(apiservererrors.RestoreError(err))
		}
		return transform.Slice(result.Results, c.resolveCharmV6), nil
	}

	var result params.ResolveCharmWithChannelResults
	if err := c.facade.FacadeCall("ResolveCharms", args, &result); err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	return transform.Slice(result.Results, c.resolveCharm), nil
}

func (c *Client) resolveCharm(r params.ResolveCharmWithChannelResult) ResolvedCharm {
	if r.Error != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(r.Error)}
	}
	curl, err := charm.ParseURL(r.URL)
	if err != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(err)}
	}
	origin, err := apicharm.APICharmOrigin(r.Origin)
	if err != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(err)}
	}

	supportedBases, err := transform.SliceOrErr(r.SupportedBases, func(in params.Base) (corebase.Base, error) {
		return corebase.ParseBase(in.Name, in.Channel)
	})
	if err != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(err)}
	}
	return ResolvedCharm{
		URL:            curl,
		Origin:         origin,
		SupportedBases: supportedBases,
	}
}

func (c *Client) resolveCharmV6(r params.ResolveCharmWithChannelResultV6) ResolvedCharm {
	if r.Error != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(r.Error)}
	}
	curl, err := charm.ParseURL(r.URL)
	if err != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(err)}
	}
	origin, err := apicharm.APICharmOrigin(r.Origin)
	if err != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(err)}
	}
	supportedBases, err := transform.SliceOrErr(r.SupportedSeries, corebase.GetBaseFromSeries)
	if err != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(err)}
	}
	return ResolvedCharm{
		URL:            curl,
		Origin:         origin,
		SupportedBases: supportedBases,
	}
}

// DownloadInfo holds the URL and Origin for a charm that requires downloading
// on the client side. This is mainly for bundles as we don't resolve bundles
// on the server.
type DownloadInfo struct {
	URL    string
	Origin apicharm.Origin
}

// GetDownloadInfo will get a download information from the given charm URL
// using the appropriate charm store.
func (c *Client) GetDownloadInfo(curl *charm.URL, origin apicharm.Origin) (DownloadInfo, error) {
	args := params.CharmURLAndOrigins{
		Entities: []params.CharmURLAndOrigin{{
			CharmURL: curl.String(),
			Origin:   origin.ParamsCharmOrigin(),
		}},
	}
	var results params.DownloadInfoResults
	if err := c.facade.FacadeCall("GetDownloadInfos", args, &results); err != nil {
		return DownloadInfo{}, errors.Trace(err)
	}
	if num := len(results.Results); num != 1 {
		return DownloadInfo{}, errors.Errorf("expected one result, received %d", num)
	}
	result := results.Results[0]
	origin, err := apicharm.APICharmOrigin(result.Origin)
	if err != nil {
		return DownloadInfo{}, errors.Trace(err)
	}
	return DownloadInfo{
		URL:    result.URL,
		Origin: origin,
	}, nil
}

// AddCharm adds the given charm URL (which must include revision) to
// the model, if it does not exist yet. Local charms are not
// supported, only charm store and charm hub URLs. See also AddLocalCharm().
//
// If the AddCharm API call fails because of an authorization error
// when retrieving the charm from the charm store, an error
// satisfying params.IsCodeUnauthorized will be returned.
func (c *Client) AddCharm(curl *charm.URL, origin apicharm.Origin, force bool) (apicharm.Origin, error) {
	args := params.AddCharmWithOrigin{
		URL:    curl.String(),
		Origin: origin.ParamsCharmOrigin(),
		Force:  force,
	}
	var result params.CharmOriginResult
	if err := c.facade.FacadeCall("AddCharm", args, &result); err != nil {
		return apicharm.Origin{}, errors.Trace(err)
	}
	return apicharm.APICharmOrigin(result.Origin)
}

// AddLocalCharm prepares the given charm with a local: schema in its
// URL, and uploads it via the API server, returning the assigned
// charm URL.
func (c *Client) AddLocalCharm(curl *charm.URL, ch charm.Charm, force bool, agentVersion version.Number) (*charm.URL, error) {
	if curl.Schema != "local" {
		return nil, errors.Errorf("expected charm URL with local: schema, got %q", curl.String())
	}

	if err := c.validateCharmVersion(ch, agentVersion); err != nil {
		return nil, errors.Trace(err)
	}
	if err := lxdprofile.ValidateLXDProfile(lxdCharmProfiler{Charm: ch}); err != nil {
		if !force {
			return nil, errors.Trace(err)
		}
	}

	// Package the charm for uploading.
	var archive *os.File
	switch ch := ch.(type) {
	case *charm.CharmDir:
		var err error
		if archive, err = os.CreateTemp("", "charm"); err != nil {
			return nil, errors.Annotate(err, "cannot create temp file")
		}
		defer func() {
			_ = archive.Close()
			_ = os.Remove(archive.Name())
		}()

		if err := ch.ArchiveTo(archive); err != nil {
			return nil, errors.Annotate(err, "cannot repackage charm")
		}
		if _, err := archive.Seek(0, 0); err != nil {
			return nil, errors.Annotate(err, "cannot rewind packaged charm")
		}
	case *charm.CharmArchive:
		var err error
		if archive, err = os.Open(ch.Path); err != nil {
			return nil, errors.Annotate(err, "cannot read charm archive")
		}
		defer archive.Close()
	default:
		return nil, errors.Errorf("unknown charm type %T", ch)
	}

	anyHooksOrDispatch, err := hasHooksOrDispatch(archive.Name())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !anyHooksOrDispatch {
		return nil, errors.Errorf("invalid charm %q: has no hooks nor dispatch file", curl.Name)
	}

	curl, err = c.uploadCharm(curl, archive)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return curl, nil
}

// uploadCharm sends the content to the API server using an HTTP post.
func (c *Client) uploadCharm(curl *charm.URL, content io.ReadSeekCloser) (*charm.URL, error) {
	args := url.Values{}
	args.Add("series", curl.Series)
	args.Add("schema", curl.Schema)
	args.Add("revision", strconv.Itoa(curl.Revision))
	apiURI := url.URL{Path: "/charms", RawQuery: args.Encode()}

	contentType := "application/zip"
	var resp params.CharmsResponse
	if err := c.httpPost(content, apiURI.String(), contentType, &resp); err != nil {
		return nil, errors.Trace(err)
	}

	curl, err := charm.ParseURL(resp.CharmURL)
	if err != nil {
		return nil, errors.Annotatef(err, "bad charm URL in response")
	}
	return curl, nil
}

func (c *Client) httpPost(content io.ReadSeeker, endpoint, contentType string, response interface{}) error {
	req, err := http.NewRequest("POST", endpoint, content)
	if err != nil {
		return errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", contentType)

	// The returned httpClient sets the base url to /model/<uuid> if it can.
	httpClient, err := c.facade.RawAPICaller().HTTPClient()
	if err != nil {
		return errors.Trace(err)
	}

	if err := httpClient.Do(c.facade.RawAPICaller().Context(), req, response); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// lxdCharmProfiler massages a charm.Charm into a LXDProfiler inside of the
// core package.
type lxdCharmProfiler struct {
	Charm charm.Charm
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p lxdCharmProfiler) LXDProfile() lxdprofile.LXDProfile {
	if p.Charm == nil {
		return nil
	}
	if profiler, ok := p.Charm.(charm.LXDProfiler); ok {
		profile := profiler.LXDProfile()
		if profile == nil {
			return nil
		}
		return profile
	}
	return nil
}

var hasHooksOrDispatch = hasHooksFolderOrDispatchFile

func hasHooksFolderOrDispatchFile(name string) (bool, error) {
	zipr, err := zip.OpenReader(name)
	if err != nil {
		return false, err
	}
	defer zipr.Close()
	count := 0
	// zip file spec 4.4.17.1 says that separators are always "/" even on Windows.
	hooksPath := "hooks/"
	dispatchPath := "dispatch"
	for _, f := range zipr.File {
		if strings.Contains(f.Name, hooksPath) {
			count++
		}
		if count > 1 {
			// 1 is the magic number here.
			// Charm zip archive is expected to contain several files and folders.
			// All properly built charms will have a non-empty "hooks" folders OR
			// a dispatch file.
			// File names in the archive will be of the form "hooks/" - for hooks folder; and
			// "hooks/*" for the actual charm hooks implementations.
			// For example, install hook may have a file with a name "hooks/install".
			// Once we know that there are, at least, 2 files that have names that start with "hooks/", we
			// know for sure that the charm has a non-empty hooks folder.
			return true, nil
		}
		if strings.Contains(f.Name, dispatchPath) {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) validateCharmVersion(ch charm.Charm, agentVersion version.Number) error {
	minver := ch.Meta().MinJujuVersion
	if minver != version.Zero {
		return jujuversion.CheckJujuMinVersion(minver, agentVersion)
	}
	return nil
}

// CheckCharmPlacement checks to see if a charm can be placed into the
// application. If the application doesn't exist then it is considered fine to
// be placed there.
func (c *Client) CheckCharmPlacement(applicationName string, curl *charm.URL) error {
	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: applicationName,
			CharmURL:    curl.String(),
		}},
	}
	var result params.ErrorResults
	if err := c.facade.FacadeCall("CheckCharmPlacement", args, &result); err != nil {
		return errors.Trace(err)
	}
	return result.OneError()
}

// ListCharmResources returns a list of associated resources for a given charm.
func (c *Client) ListCharmResources(curl *charm.URL, origin apicharm.Origin) ([]charmresource.Resource, error) {
	args := params.CharmURLAndOrigins{
		Entities: []params.CharmURLAndOrigin{{
			CharmURL: curl.String(),
			Origin:   origin.ParamsCharmOrigin(),
		}},
	}
	var results params.CharmResourcesResults
	if err := c.facade.FacadeCall("ListCharmResources", args, &results); err != nil {
		return nil, errors.Trace(err)
	}

	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, received %d", n)
	}

	result := results.Results[0]
	resources := make([]charmresource.Resource, len(result))
	for i, res := range result {
		if res.Error != nil {
			return nil, errors.Trace(res.Error)
		}

		chRes, err := api.API2CharmResource(res.CharmResource)
		if err != nil {
			return nil, errors.Annotate(err, "unexpected charm resource")
		}
		resources[i] = chRes
	}

	return resources, nil
}
