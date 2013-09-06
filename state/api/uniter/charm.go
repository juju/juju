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

func (c *Charm) getBundleInfo(apiCall string) (string, error) {
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

// BundleURL returns the url to the charm bundle in
// the provider storage.
//
// NOTE: This differs from state.Charm.BundleURL() by returning an
// error as well, because it needs to make an API call
func (c *Charm) BundleURL() (*url.URL, error) {
	charmURL, err := c.getBundleInfo("CharmBundleURL")
	if err != nil {
		return nil, err
	}
	return url.Parse(charmURL)
}

// BundleSha256 returns the SHA256 digest of the charm bundle bytes.
//
// NOTE: This differs from state.Charm.BundleSha256() by returning an
// error as well, because it needs to make an API call
func (c *Charm) BundleSha256() (string, error) {
	return c.getBundleInfo("CharmBundleSha256")
}
