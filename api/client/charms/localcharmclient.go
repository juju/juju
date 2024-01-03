// Copyright 2023 Canonical Ltd.
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
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
)

// LocalCharmClient allows access to the API endpoints
// required to add a local charm
type LocalCharmClient struct {
	base.ClientFacade
	facade base.FacadeCaller
}

func NewLocalCharmClient(st base.APICallCloser) *LocalCharmClient {
	frontend, backend := base.NewClientFacade(st, "Charms")
	return &LocalCharmClient{ClientFacade: frontend, facade: backend}
}

// AddLocalCharm prepares the given charm with a local: schema in its
// URL, and uploads it via the API server, returning the assigned
// charm URL.
func (c *LocalCharmClient) AddLocalCharm(curl *charm.URL, ch charm.Charm, force bool, agentVersion version.Number) (*charm.URL, error) {
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
func (c *LocalCharmClient) uploadCharm(curl *charm.URL, content io.ReadSeekCloser) (*charm.URL, error) {
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

func (c *LocalCharmClient) httpPost(content io.ReadSeeker, endpoint, contentType string, response interface{}) error {
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

func (c *LocalCharmClient) validateCharmVersion(ch charm.Charm, agentVersion version.Number) error {
	minver := ch.Meta().MinJujuVersion
	if minver != version.Zero {
		return jujuversion.CheckJujuMinVersion(minver, agentVersion)
	}
	return nil
}
