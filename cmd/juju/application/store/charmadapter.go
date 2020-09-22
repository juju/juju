// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"

	apicharm "github.com/juju/juju/api/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
)

// CharmAdaptor handles prep work for deploying charms: resolving charms
// and bundles and getting bundle contents.  This is done via the charmstore
// or the charms API depending on the API's version.
type CharmAdaptor struct {
	charmrepo        CharmrepoForDeploy
	charmsAPIVersion int
	charmsAPI        CharmsAPI
}

// NewCharmAdaptor returns a CharmAdaptor.
func NewCharmAdaptor(charmrepo CharmrepoForDeploy, charmsAPIVersion int, charmsAPI CharmsAPI) *CharmAdaptor {
	return &CharmAdaptor{
		charmsAPIVersion: charmsAPIVersion,
		charmrepo:        charmrepo,
		charmsAPI:        charmsAPI,
	}
}

// ResolveCharm tries to interpret url as a CharmStore or CharmHub charm.
// If it turns out to be one of those charm types, the resolved URL, origin
// and a slice of supported series are returned.
// Resolving a CharmHub charm is only supported if the controller has a
// Charms API version of 3 or greater.
func (c *CharmAdaptor) ResolveCharm(url *charm.URL, preferredOrigin commoncharm.Origin) (*charm.URL, commoncharm.Origin, []string, error) {
	if c.charmsAPIVersion >= 3 {
		resolved, err := c.charmsAPI.ResolveCharms([]apicharm.CharmToResolve{{URL: url, Origin: preferredOrigin}})
		if err != nil {
			return nil, commoncharm.Origin{}, nil, err
		}
		return resolved[0].URL, resolved[0].Origin, resolved[0].SupportedSeries, resolved[0].Error
	}

	if url.Schema != "cs" {
		return nil, commoncharm.Origin{}, nil, errors.Errorf("unknown schema for charm URL %q", url)
	}

	resultURL, channel, supportedSeries, err := c.charmrepo.ResolveWithPreferredChannel(url, csparams.Channel(preferredOrigin.Risk))
	if err != nil {
		return nil, commoncharm.Origin{}, nil, errors.Trace(err)
	}
	origin := preferredOrigin
	origin.Risk = string(channel)
	if resultURL.Series != "" && len(supportedSeries) == 0 {
		supportedSeries = []string{resultURL.Series}
	}
	return resultURL, origin, supportedSeries, nil
}

// ResolveBundleURL tries to interpret maybeBundle as a CharmStore
// or CharmHub bundle. If it turns out to be a bundle, the resolved
// URL and origin are returned. If it isn't but there wasn't a problem
// checking it, it returns a nil charm URL.
func (c *CharmAdaptor) ResolveBundleURL(maybeBundle *charm.URL, preferredOrigin commoncharm.Origin) (*charm.URL, commoncharm.Origin, error) {
	// Charm or bundle has been supplied as a URL so we resolve and
	// deploy using the store.
	storeCharmOrBundleURL, origin, _, err := c.ResolveCharm(maybeBundle, preferredOrigin)
	if err != nil {
		return nil, commoncharm.Origin{}, errors.Trace(err)
	}
	if storeCharmOrBundleURL.Series != "bundle" {
		logger.Debugf(
			`cannot interpret as charmstore bundle: %v (series) != "bundle"`,
			storeCharmOrBundleURL.Series,
		)
		return nil, commoncharm.Origin{}, errors.NotValidf("charmstore bundle %q", maybeBundle)
	}
	return storeCharmOrBundleURL, origin, nil
}

func (c *CharmAdaptor) GetBundle(bundleURL *charm.URL, path string) (charm.Bundle, error) {
	// TODO (hml) 2020-08-25
	// Implement the CharmsAPI version for this.
	return c.charmrepo.GetBundle(bundleURL, path)
}
