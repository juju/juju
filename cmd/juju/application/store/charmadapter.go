// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	"net/url"

	"github.com/juju/charm/v9"
	csparams "github.com/juju/charmrepo/v7/csclient/params"
	"github.com/juju/errors"

	apicharm "github.com/juju/juju/api/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
)

// CharmStoreRepoFunc lazily creates a charm store repo.
type CharmStoreRepoFunc = func() (CharmrepoForDeploy, error)

// DownloadBundleClient represents a way to download a bundle from a given
// resource URL.
type DownloadBundleClient interface {
	DownloadAndReadBundle(context.Context, *url.URL, string, ...charmhub.DownloadOption) (charm.Bundle, error)
}

// DownloadBundleClientFunc lazily construct a download bundle client.
type DownloadBundleClientFunc = func() (DownloadBundleClient, error)

// BundleFactory represents a type for getting a bundle from a given url.
type BundleFactory interface {
	GetBundle(*charm.URL, commoncharm.Origin, string) (charm.Bundle, error)
}

// BundleRepoFunc creates a bundle factory from a charm URL.
type BundleRepoFunc = func(*charm.URL) (BundleFactory, error)

// CharmAdaptor handles prep work for deploying charms: resolving charms
// and bundles and getting bundle contents.  This is done via the charmstore
// or the charms API depending on the API's version.
type CharmAdaptor struct {
	charmsAPI          CharmsAPI
	charmStoreRepoFunc CharmStoreRepoFunc
	bundleRepoFn       BundleRepoFunc
}

// NewCharmAdaptor returns a CharmAdaptor.
func NewCharmAdaptor(charmsAPI CharmsAPI, charmStoreRepoFunc CharmStoreRepoFunc, downloadBundleClientFunc DownloadBundleClientFunc) *CharmAdaptor {
	return &CharmAdaptor{
		charmsAPI:          charmsAPI,
		charmStoreRepoFunc: charmStoreRepoFunc,

		bundleRepoFn: func(url *charm.URL) (BundleFactory, error) {
			if charm.CharmHub.Matches(url.Schema) {
				return chBundleFactory{
					charmsAPI:                charmsAPI,
					downloadBundleClientFunc: downloadBundleClientFunc,
				}, nil
			}
			return csBundleFactory{
				charmStoreRepoFunc: charmStoreRepoFunc,
			}, nil
		},
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
	charmRepo, err := c.charmStoreRepoFunc()
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
	if transport.BundleType.Matches(origin.Type) || storeCharmOrBundleURL.Series == "bundle" {
		return storeCharmOrBundleURL, origin, nil
	}

	logger.Debugf(
		`cannot interpret as charmstore bundle: %v (series) != "bundle"`,
		storeCharmOrBundleURL.Series,
	)
	return nil, commoncharm.Origin{}, errors.NotValidf("charmstore bundle %q", maybeBundle)
}

// GetBundle returns a bundle from a given charmstore path.
func (c *CharmAdaptor) GetBundle(url *charm.URL, origin commoncharm.Origin, path string) (charm.Bundle, error) {
	repo, err := c.bundleRepoFn(url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return repo.GetBundle(url, origin, path)
}

type csBundleFactory struct {
	charmStoreRepoFunc CharmStoreRepoFunc
}

func (cs csBundleFactory) GetBundle(url *charm.URL, _ commoncharm.Origin, path string) (charm.Bundle, error) {
	charmRepo, err := cs.charmStoreRepoFunc()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return charmRepo.GetBundle(url, path)
}

type chBundleFactory struct {
	charmsAPI                CharmsAPI
	downloadBundleClientFunc DownloadBundleClientFunc
}

func (ch chBundleFactory) GetBundle(curl *charm.URL, origin commoncharm.Origin, path string) (charm.Bundle, error) {
	client, err := ch.downloadBundleClientFunc()
	if err != nil {
		return nil, errors.Trace(err)
	}

	info, err := ch.charmsAPI.GetDownloadInfo(curl, origin, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	url, err := url.Parse(info.URL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client.DownloadAndReadBundle(context.TODO(), url, path)
}
