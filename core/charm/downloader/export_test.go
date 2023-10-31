// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"context"

	"github.com/juju/charm/v11"

	corecharm "github.com/juju/juju/core/charm"
)

func (dc DownloadedCharm) Verify(downloadOrigin corecharm.Origin, force bool) error {
	return dc.verify(downloadOrigin, force)
}

func (d *Downloader) NormalizePlatform(charmURL string, platform corecharm.Platform) (corecharm.Platform, error) {
	return d.normalizePlatform(charmURL, platform)
}

func (d *Downloader) DownloadAndHash(ctx context.Context, charmURL *charm.URL, requestedOrigin corecharm.Origin, repo CharmRepository, dstPath string) (DownloadedCharm, corecharm.Origin, error) {
	return d.downloadAndHash(ctx, charmURL, requestedOrigin, repo, dstPath)
}
