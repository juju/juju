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

type CharmRepoFunc = func() (CharmrepoForDeploy, error)

// CharmAdaptor handles prep work for deploying charms: resolving charms
// and bundles and getting bundle contents.  This is done via the charmstore
// or the charms API depending on the API's version.
type CharmAdaptor struct {
	charmsAPI   CharmsAPI
	charmRepoFn CharmRepoFunc
}

// NewCharmAdaptor returns a CharmAdaptor.
func NewCharmAdaptor(charmsAPI CharmsAPI, charmRepoFn CharmRepoFunc) *CharmAdaptor {
	return &CharmAdaptor{
		charmsAPI:   charmsAPI,
		charmRepoFn: charmRepoFn,
	}
}

// ResolveCharm tries to interpret url as a CharmStore or CharmHub charm.
// If it turns out to be one of those charm types, the resolved URL, origin
// and a slice of supported series are returned.
// Resolving a CharmHub charm is only supported if the controller has a
// Charms API version of 3 or greater.
func (c *CharmAdaptor) ResolveCharm(url *charm.URL, preferredOrigin commoncharm.Origin) (*charm.URL, commoncharm.Origin, []string, error) {
	resolved, err := c.charmsAPI.ResolveCharms([]apicharm.CharmToResolve{{URL: url, Origin: preferredOrigin}})
	if errors.IsNotSupported(err) {
		if charm.CharmHub.Matches(url.Schema) {
			return nil, commoncharm.Origin{}, nil, errors.Trace(err)
		}
		return c.resolveCharmFallback(url, preferredOrigin)
	}
	if err != nil {
		return nil, commoncharm.Origin{}, nil, errors.Trace(err)
	}
	return resolved[0].URL, resolved[0].Origin, resolved[0].SupportedSeries, resolved[0].Error
}

func (c *CharmAdaptor) resolveCharmFallback(url *charm.URL, preferredOrigin commoncharm.Origin) (*charm.URL, commoncharm.Origin, []string, error) {
	charmRepo, err := c.charmRepoFn()
	if err != nil {
		return nil, commoncharm.Origin{}, nil, errors.Trace(err)
	}

	resultURL, channel, supportedSeries, err := charmRepo.ResolveWithPreferredChannel(url, csparams.Channel(preferredOrigin.Risk))
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
	// We're a bundle so return out before handling the invalid flow.
	if origin.Type == "bundle" || storeCharmOrBundleURL.Series == "bundle" {
		return storeCharmOrBundleURL, origin, nil
	}

	logger.Debugf(
		`cannot interpret as charmstore bundle: %v (series) != "bundle"`,
		storeCharmOrBundleURL.Series,
	)
	return nil, commoncharm.Origin{}, errors.NotValidf("charmstore bundle %q", maybeBundle)
}

// GetBundle returns a bundle from a given charmstore path.
func (c *CharmAdaptor) GetBundle(bundleURL *charm.URL, path string) (charm.Bundle, error) {
	// TODO (hml) 2020-08-25
	// Implement the CharmsAPI version for this.
	charmRepo, err := c.charmRepoFn()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return charmRepo.GetBundle(bundleURL, path)
}
