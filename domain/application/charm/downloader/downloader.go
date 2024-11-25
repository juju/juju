// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"context"
	"os"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charmhub"
)

// CharmRepository provides an API for downloading charms/bundles.
type CharmRepository interface {
	// Download downloads a blob with the specified name and origin to the
	// specified path. The origin is used to determine the source of the blob
	// and the channel to use when downloading the blob.
	Download(ctx context.Context, blobName string, requestedOrigin corecharm.Origin, archivePath string) (corecharm.Origin, *charmhub.Digest, error)
}

// RepositoryGetter returns a suitable CharmRepository for the specified Source.
type RepositoryGetter interface {
	GetCharmRepository(context.Context, corecharm.Source) (CharmRepository, error)
}

// DownloadResult contains information about a downloaded charm.
type DownloadResult struct {
	Path   string
	Origin corecharm.Origin
	Size   int64
}

// CharmDownloader implements store-agnostic download and persistence of charm
// blobs.
type CharmDownloader struct {
	repoGetter RepositoryGetter
	logger     logger.Logger
}

// NewCharmDownloader returns a new charm downloader instance.
func NewCharmDownloader(repoGetter RepositoryGetter, logger logger.Logger) *CharmDownloader {
	return &CharmDownloader{
		repoGetter: repoGetter,
		logger:     logger,
	}
}

// Download looks up the requested charm using the appropriate store, downloads
// it to a temporary file and passes it to the configured storage API so it can
// be persisted.
//
// The resulting charm is verified to be the right hash. It expected that the
// origin will always have the correct hash following this call.
func (d *CharmDownloader) Download(ctx context.Context, name string, requestedOrigin corecharm.Origin) (*DownloadResult, error) {
	channelOrigin := requestedOrigin

	var err error
	channelOrigin.Platform, err = d.normalizePlatform(name, requestedOrigin.Platform)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Get the repository for the requested origin source. Allowing us to
	// switch between different charm sources (charmhub).
	repo, err := d.getRepo(ctx, requestedOrigin.Source)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Download the charm from the repository and hash it.
	result, err := d.download(ctx, name, channelOrigin, repo)
	if err != nil {
		return nil, errors.Annotatef(err, "downloading %q using origin %v", name, requestedOrigin)
	}

	return result, nil
}

func (d *CharmDownloader) getRepo(ctx context.Context, src corecharm.Source) (CharmRepository, error) {
	repo, err := d.repoGetter.GetCharmRepository(ctx, src)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return repo, nil
}

func (d *CharmDownloader) download(ctx context.Context, name string, requestedOrigin corecharm.Origin, repo CharmRepository) (*DownloadResult, error) {
	d.logger.Debugf("downloading charm %q using origin %v", name, requestedOrigin)

	tmpFile, err := os.CreateTemp("", name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer tmpFile.Close()

	actualOrigin, digest, err := repo.Download(ctx, name, requestedOrigin, tmpFile.Name())
	if err != nil {
		// Ensure we cleanup if the download fails.
		_ = tmpFile.Close()

		return nil, errors.Trace(err)
	}

	// We expect that after downloading the result is verified.
	if digest.Hash != requestedOrigin.Hash {
		return nil, errors.Errorf("expected hash %q, got %q", requestedOrigin.Hash, digest.Hash)
	}

	d.logger.Debugf("downloaded charm %q from actual origin %v", name, actualOrigin)

	return &DownloadResult{
		Path:   tmpFile.Name(),
		Origin: actualOrigin,
		Size:   digest.Size,
	}, nil
}

func (d *CharmDownloader) normalizePlatform(charmName string, platform corecharm.Platform) (corecharm.Platform, error) {
	arc := platform.Architecture
	if platform.Architecture == "" || platform.Architecture == "all" {
		d.logger.Warningf("received charm Architecture: %q, changing to %q, for charm %q", platform.Architecture, arch.DefaultArchitecture, charmName)
		arc = arch.DefaultArchitecture
	}

	// Values passed to the api are case sensitive: ubuntu succeeds and
	// Ubuntu returns `"code": "revision-not-found"`
	return corecharm.Platform{
		Architecture: arc,
		OS:           strings.ToLower(platform.OS),
		Channel:      platform.Channel,
	}, nil
}
