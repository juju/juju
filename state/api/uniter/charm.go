// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"net/url"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state/api/params"
)

// This module implements a subset of the interface provided by
// state.Charm, as needed by the uniter API.

// Charm represents the state of a charm in the environment.
type Charm struct {
	st  *State
	url string
}

// Strings returns the charm URL as a string.
func (c *Charm) String() string {
	return c.url
}

// URL returns the URL that identifies the charm.
func (c *Charm) URL() *charm.URL {
	return charm.MustParseURL(c.url)
}

func (c *Charm) getArchiveInfo(apiCall string) (string, error) {
	var results params.StringResults
	args := params.CharmURLs{
		URLs: []params.CharmURL{{URL: c.url}},
	}
	err := c.st.caller.Call("Uniter", "", apiCall, args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// ArchiveURL returns the url to the charm archive (bundle) in the
// provider storage, and DisableSSLHostnameVerification flag.
//
// NOTE: This differs from state.Charm.BundleURL() by returning an
// error as well, because it needs to make an API call. It's also
// renamed to avoid confusion with juju deployment bundles.
//
// TODO(dimitern): 2013-09-06 bug 1221834
// Cache the result after getting it once for the same charm URL,
// because it's immutable.
func (c *Charm) ArchiveURL() (*url.URL, bool, error) {
	var results params.CharmArchiveURLResults
	args := params.CharmURLs{
		URLs: []params.CharmURL{{URL: c.url}},
	}
	err := c.st.caller.Call("Uniter", "", "CharmArchiveURL", args, &results)
	if err != nil {
		return nil, false, err
	}
	if len(results.Results) != 1 {
		return nil, false, fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, false, result.Error
	}
	archiveURL, err := url.Parse(result.Result)
	if err != nil {
		return nil, false, err
	}
	return archiveURL, result.DisableSSLHostnameVerification, nil
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
