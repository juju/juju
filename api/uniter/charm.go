// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"net/url"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/params"
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

// ArchiveURLs returns the URLs to the charm archive (bundle) in the
// environment storage. Each URL should be tried until one succeeds.
func (c *Charm) ArchiveURLs() ([]*url.URL, error) {
	var results params.StringsResults
	args := params.CharmURLs{
		URLs: []params.CharmURL{{URL: c.curl.String()}},
	}
	err := c.st.facade.FacadeCall("CharmArchiveURLs", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	archiveURLs := make([]*url.URL, len(result.Result))
	for i, rawurl := range result.Result {
		archiveURL, err := url.Parse(rawurl)
		if err != nil {
			return nil, errors.Annotate(err, "server returned an invalid URL")
		}
		archiveURLs[i] = archiveURL
	}
	return archiveURLs, nil
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
	var results params.StringResults
	args := params.CharmURLs{
		URLs: []params.CharmURL{{URL: c.curl.String()}},
	}
	err := c.st.facade.FacadeCall("CharmArchiveSha256", args, &results)
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
