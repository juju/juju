// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	stderrors "errors"
	"net"
	"net/url"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"

	apicharm "github.com/juju/juju/api/client/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
)

const (
	bundleDownloadRetryAttempts = 3
	bundleDownloadRetryDelay    = 20 * time.Second
)

// DownloadBundleClient represents a way to download a bundle from a given
// resource URL.
type DownloadBundleClient interface {
	Download(context.Context, *url.URL, string, ...charmhub.DownloadOption) (*charmhub.Digest, error)
}

// DownloadBundleClientFunc lazily construct a download bundle client.
type DownloadBundleClientFunc = func(ctx context.Context) (DownloadBundleClient, error)

// BundleFactory represents a type for getting a bundle from a given url.
type BundleFactory interface {
	GetBundle(context.Context, *charm.URL, commoncharm.Origin, string) (charm.Bundle, error)
}

// CharmReader represents a type for reading a bundle archive.
type CharmReader interface {
	ReadBundleArchive(path string) (charm.Bundle, error)
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
				charmReader:              charmReader{},
				downloadBundleClientFunc: downloadBundleClientFunc,
				downloadRetryClock:       clock.WallClock,
				downloadRetryDelay:       bundleDownloadRetryDelay,
			}, nil
		},
	}
}

// ResolveCharm tries to interpret url as a Charmhub charm and
// returns the resolved URL, origin and a slice of supported series.
func (c *CharmAdaptor) ResolveCharm(ctx context.Context, url *charm.URL, preferredOrigin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []base.Base, error) {
	resolved, err := c.charmsAPI.ResolveCharms(ctx, []apicharm.CharmToResolve{{URL: url, Origin: preferredOrigin, SwitchCharm: switchCharm}})
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
func (c *CharmAdaptor) ResolveBundleURL(ctx context.Context, maybeBundle *charm.URL, preferredOrigin commoncharm.Origin) (*charm.URL, commoncharm.Origin, error) {
	// Charm or bundle has been supplied as a URL so we resolve and
	// deploy using the store. In this case, a --switch is not possible
	// so we pass "false" to ResolveCharm.
	storeCharmOrBundleURL, origin, _, err := c.ResolveCharm(ctx, maybeBundle, preferredOrigin, false)
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

type charmReader struct{}

func (charmReader) ReadBundleArchive(path string) (charm.Bundle, error) {
	return charm.ReadBundleArchive(path)
}

type chBundleFactory struct {
	charmsAPI                CharmsAPI
	charmReader              CharmReader
	downloadBundleClientFunc DownloadBundleClientFunc
	downloadRetryClock       clock.Clock
	downloadRetryDelay       time.Duration
}

func (ch chBundleFactory) GetBundle(ctx context.Context, curl *charm.URL, origin commoncharm.Origin, path string) (charm.Bundle, error) {
	client, err := ch.downloadBundleClientFunc(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	info, err := ch.charmsAPI.GetDownloadInfo(ctx, curl, origin)
	if err != nil {
		return nil, errors.Trace(err)
	}
	url, err := url.Parse(info.URL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := ch.downloadBundle(ctx, client, url, path); err != nil {
		return nil, errors.Trace(err)
	}
	bundle, err := ch.charmReader.ReadBundleArchive(path)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return bundle, nil
}

func (ch chBundleFactory) downloadBundle(ctx context.Context, client DownloadBundleClient, url *url.URL, path string) error {
	return retry.Call(retry.CallArgs{
		Func: func() error {
			_, err := client.Download(ctx, url, path)
			if err != nil {
				return errors.Trace(err)
			}
			return nil
		},
		IsFatalError: func(err error) bool {
			return !isTransientBundleDownloadError(err)
		},
		Attempts: bundleDownloadRetryAttempts,
		Clock:    ch.effectiveDownloadRetryClock(),
		Delay:    ch.effectiveDownloadRetryDelay(),
		Stop:     ctx.Done(),
	})
}

func (ch chBundleFactory) effectiveDownloadRetryClock() clock.Clock {
	if ch.downloadRetryClock != nil {
		return ch.downloadRetryClock
	}
	return clock.WallClock
}

func (ch chBundleFactory) effectiveDownloadRetryDelay() time.Duration {
	if ch.downloadRetryDelay > 0 {
		return ch.downloadRetryDelay
	}
	return bundleDownloadRetryDelay
}

func isTransientBundleDownloadError(err error) bool {
	if err == nil {
		return false
	}
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// TODO (stickupkid): We can add more transient error types here as we
	// identify them, for example, errors related to temporary network issues.

	var netErr net.Error
	if stderrors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}
