// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"net/url"
	"path"

	"gopkg.in/juju/charm.v3"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/names"
)

// This module implements a subset of the interface provided by
// state.Charm, as needed by the uniter API.

// Charm represents the state of a charm in the environment.
type Charm struct {
	st   *State
	curl *charm.URL
}

// String returns the charm URL as a string.
func (c *Charm) String() string {
	return c.curl.String()
}

// URL returns the URL that identifies the charm.
func (c *Charm) URL() *charm.URL {
	return c.curl
}

func (c *Charm) getArchiveInfo(apiCall string) (string, error) {
	var results params.StringResults
	args := params.CharmURLs{
		URLs: []params.CharmURL{{URL: c.curl.String()}},
	}
	err := c.st.facade.FacadeCall(apiCall, args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// ArchiveURL returns the url to the charm archive (bundle) in the
// environment storage.
func (c *Charm) ArchiveURL() *url.URL {
	archiveURL := *c.st.charmsURL
	q := archiveURL.Query()
	q.Set("url", c.curl.String())
	q.Set("file", "*")
	archiveURL.RawQuery = q.Encode()
	return &archiveURL
}

// ArchiveSha256 returns the SHA256 digest of the charm archive
// (bundle) bytes.
//
// NOTE: This differs from state.Charm.BundleSha256() by returning an
// error as well, because it needs to make an API call. It's also
// renamed to avoid confusion with juju deployment bundles.
//
// TODO(dimitern): 2013-09-06 bug 1221834
// Cache the result after getting it once for the same charm URL,
// because it's immutable.
func (c *Charm) ArchiveSha256() (string, error) {
	return c.getArchiveInfo("CharmArchiveSha256")
}

// CharmsURL takes an API server address and an optional environment
// tag and constructs a base URL used for fetching charm archives.
// If the environment tag is omitted or invalid, it will be ignored.
func CharmsURL(apiAddr string, envTag string) *url.URL {
	urlPath := "/"
	if envTag != "" {
		tag, err := names.ParseEnvironTag(envTag)
		if err == nil {
			urlPath = path.Join(urlPath, "environment", tag.Id())
		}
	}
	urlPath = path.Join(urlPath, "charms")
	return &url.URL{Scheme: "https", Host: apiAddr, Path: urlPath}
}
