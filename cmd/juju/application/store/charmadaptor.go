// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	"net/url"

	"github.com/juju/charm/v13"
	"github.com/juju/errors"

	apicharm "github.com/juju/juju/api/client/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
)

// DownloadBundleClient represents a way to download a bundle from a given
// resource URL.
type DownloadBundleClient interface {
	DownloadAndReadBundle(context.Context, *url.URL, string, ...charmhub.DownloadOption) (charm.Bundle, error)
}

// DownloadBundleClientFunc lazily construct a download bundle client.
type DownloadBundleClientFunc = func() (DownloadBundleClient, error)

// BundleFactory represents a type for getting a bundle from a given url.
type BundleFactory interface {
	GetBundle(context.Context, *charm.URL, commoncharm.Origin, string) (charm.Bundle, error)
}

// BundleRepoFunc creates a bundle factory from a charm URL.
type BundleRepoFunc = func(*charm.URL) (BundleFactory, error)

// CharmAdaptor handles prep work for deploying charms: resolving charms
// and bundles and getting bundle contents.
type CharmAdaptor struct {
	charmsAPI    CharmsAPI
	bundleRepoFn BundleRepoFunc
}

// NewCharmAdaptor returns a CharmAdaptor.
func NewCharmAdaptor(charmsAPI CharmsAPI, downloadBundleClientFunc DownloadBundleClientFunc) *CharmAdaptor {
	return &CharmAdaptor{
		charmsAPI: charmsAPI,
		bundleRepoFn: func(url *charm.URL) (BundleFactory, error) {
			return chBundleFactory{
				charmsAPI:                charmsAPI,
				downloadBundleClientFunc: downloadBundleClientFunc,
			}, nil
		},
	}
}

// ResolveCharm tries to interpret url as a Charmhub charm and
// returns the resolved URL, origin and a slice of supported series.
func (c *CharmAdaptor) ResolveCharm(url *charm.URL, preferredOrigin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []base.Base, error) {
	resolved, err := c.charmsAPI.ResolveCharms([]apicharm.CharmToResolve{{URL: url, Origin: preferredOrigin, SwitchCharm: switchCharm}})
	if err != nil {
		return nil, commoncharm.Origin{}, nil, errors.Trace(err)
	}
	if len(resolved) == 0 {
		return nil, commoncharm.Origin{}, nil, errors.NotFoundf(url.Name)
	}
	if err := resolved[0].Error; err != nil {
		return nil, commoncharm.Origin{}, nil, errors.Trace(err)
	}

	res := resolved[0]
	return res.URL, res.Origin, res.SupportedBases, nil
}

// ResolveBundleURL tries to interpret maybeBundle as a Charmhub
// bundle. If it turns out to be a bundle, the resolved
// URL and origin are returned. If it isn't but there wasn't a problem
// checking it, it returns a nil charm URL.
func (c *CharmAdaptor) ResolveBundleURL(maybeBundle *charm.URL, preferredOrigin commoncharm.Origin) (*charm.URL, commoncharm.Origin, error) {
	// Charm or bundle has been supplied as a URL so we resolve and
	// deploy using the store. In this case, a --switch is not possible
	// so we pass "false" to ResolveCharm.
	storeCharmOrBundleURL, origin, _, err := c.ResolveCharm(maybeBundle, preferredOrigin, false)
	if err != nil {
		return nil, commoncharm.Origin{}, errors.Trace(err)
	}
	// We're a bundle so return out before handling the invalid flow.
	if transport.BundleType.Matches(origin.Type) {
		return storeCharmOrBundleURL, origin, nil
	}

	return nil, commoncharm.Origin{}, errors.NotValidf("charmstore bundle %q", maybeBundle)
}

// GetBundle returns a bundle from a given charmstore path.
func (c *CharmAdaptor) GetBundle(ctx context.Context, url *charm.URL, origin commoncharm.Origin, path string) (charm.Bundle, error) {
	repo, err := c.bundleRepoFn(url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return repo.GetBundle(ctx, url, origin, path)
}

type chBundleFactory struct {
	charmsAPI                CharmsAPI
	downloadBundleClientFunc DownloadBundleClientFunc
}

func (ch chBundleFactory) GetBundle(ctx context.Context, curl *charm.URL, origin commoncharm.Origin, path string) (charm.Bundle, error) {
	client, err := ch.downloadBundleClientFunc()
	if err != nil {
		return nil, errors.Trace(err)
	}

	info, err := ch.charmsAPI.GetDownloadInfo(curl, origin)
	if err != nil {
		return nil, errors.Trace(err)
	}
	url, err := url.Parse(info.URL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client.DownloadAndReadBundle(ctx, url, path)
}
