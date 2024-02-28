// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"context"

	corecharm "github.com/juju/juju/core/charm"
)

func (dc DownloadedCharm) Verify(downloadOrigin corecharm.Origin, force bool) error {
	return dc.verify(downloadOrigin, force)
}

func (d *Downloader) NormalizePlatform(charmURL string, platform corecharm.Platform) (corecharm.Platform, error) {
	return d.normalizePlatform(charmURL, platform)
}

func (d *Downloader) DownloadAndHash(ctx context.Context, charmName string, requestedOrigin corecharm.Origin, repo CharmRepository, dstPath string) (DownloadedCharm, corecharm.Origin, error) {
	return d.downloadAndHash(ctx, charmName, requestedOrigin, repo, dstPath)
}
