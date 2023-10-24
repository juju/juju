// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"context"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/version"
)

// Logger defines the logging methods that the package uses.
type Logger interface {
	Tracef(string, ...interface{})
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}

// CharmArchive provides information about a downloaded charm archive.
type CharmArchive interface {
	corecharm.CharmArchive
}

// CharmRepository provides an API for downloading charms/bundles.
type CharmRepository interface {
	GetDownloadURL(*charm.URL, corecharm.Origin) (*url.URL, corecharm.Origin, error)
	ResolveWithPreferredChannel(charmURL *charm.URL, requestedOrigin corecharm.Origin) (*charm.URL, corecharm.Origin, []corecharm.Platform, error)
	DownloadCharm(charmURL *charm.URL, requestedOrigin corecharm.Origin, archivePath string) (corecharm.CharmArchive, corecharm.Origin, error)
}

// RepositoryGetter returns a suitable CharmRepository for the specified Source.
type RepositoryGetter interface {
	GetCharmRepository(corecharm.Source) (CharmRepository, error)
}

// Storage provides an API for storing downloaded charms.
type Storage interface {
	PrepareToStoreCharm(string) error
	Store(context.Context, string, DownloadedCharm) error
}

// DownloadedCharm encapsulates the details of a downloaded charm.
type DownloadedCharm struct {
	// Charm provides information about the charm contents.
	Charm charm.Charm

	// The charm version.
	CharmVersion string

	// CharmData provides byte-level access to the downloaded charm data.
	CharmData io.Reader

	// The Size of the charm data in bytes.
	Size int64

	// SHA256 is the hash of the bytes in Data.
	SHA256 string

	// The LXD profile or nil if no profile specified by the charm.
	LXDProfile *charm.LXDProfile
}

// verify checks that the charm is compatible with the specified Juju version
// and ensure that the LXDProfile (if one is specified) is valid.
func (dc DownloadedCharm) verify(downloadOrigin corecharm.Origin, force bool) error {
	if err := version.CheckJujuMinVersion(dc.Charm.Meta().MinJujuVersion, version.Current); err != nil {
		return errors.Trace(err)
	}

	if dc.LXDProfile != nil {
		if err := lxdprofile.ValidateLXDProfile(lxdProfiler{dc.LXDProfile}); err != nil && !force {
			return errors.Annotate(err, "cannot verify charm-provided LXD profile")
		}
	}

	if downloadOrigin.Hash != "" && downloadOrigin.Hash != dc.SHA256 {
		return errors.NewNotValid(nil, "detected SHA256 hash mismatch")
	}

	return nil
}

// Downloader implements store-agnostic download and pesistence of charm blobs.
type Downloader struct {
	logger     Logger
	repoGetter RepositoryGetter
	storage    Storage
}

// NewDownloader returns a new charm downloader instance.
func NewDownloader(logger Logger, storage Storage, repoGetter RepositoryGetter) *Downloader {
	return &Downloader{
		repoGetter: repoGetter,
		storage:    storage,
		logger:     logger,
	}
}

// DownloadAndStore looks up the requested charm using the appropriate store,
// downloads it to a temporary file and passes it to the configured storage
// API so it can be persisted.
//
// The method ensures that all temporary resources are cleaned up before returning.
func (d *Downloader) DownloadAndStore(ctx context.Context, charmURL *charm.URL, requestedOrigin corecharm.Origin, force bool) (corecharm.Origin, error) {
	var (
		err           error
		channelOrigin = requestedOrigin
	)
	channelOrigin.Platform, err = d.normalizePlatform(charmURL.String(), requestedOrigin.Platform)
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}

	// Notify the storage layer that we are preparing to upload a charm.
	if err := d.storage.PrepareToStoreCharm(charmURL.String()); err != nil {
		// The charm blob is already uploaded this is a no-op. However,
		// as the original origin might be different that the one
		// requested by the caller, make sure to resolve it again.
		if alreadyUploadedErr, valid := errors.Cause(err).(errCharmAlreadyStored); valid {
			d.logger.Debugf("%v", alreadyUploadedErr)

			repo, err := d.getRepo(requestedOrigin.Source)
			if err != nil {
				return corecharm.Origin{}, errors.Trace(err)
			}
			_, resolvedOrigin, err := repo.GetDownloadURL(charmURL, requestedOrigin)
			return resolvedOrigin, errors.Trace(err)
		}

		return corecharm.Origin{}, errors.Trace(err)
	}

	// Download charm blob to a temp file
	tmpFile, err := os.CreateTemp("", charmURL.Name)
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}
	defer func() {
		_ = tmpFile.Close()
		if err := os.Remove(tmpFile.Name()); err != nil {
			d.logger.Warningf("unable to remove temporary charm download path %q", tmpFile.Name())
		}
	}()

	repo, err := d.getRepo(requestedOrigin.Source)
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}

	downloadedCharm, actualOrigin, err := d.downloadAndHash(charmURL, channelOrigin, repo, tmpFile.Name())
	if err != nil {
		return corecharm.Origin{}, errors.Annotatef(err, "downloading charm %q from origin %v", charmURL, requestedOrigin)
	}

	// Validate charm
	if err := downloadedCharm.verify(actualOrigin, force); err != nil {
		return corecharm.Origin{}, errors.Annotatef(err, "verifying downloaded charm %q from origin %v", charmURL, requestedOrigin)
	}

	// Store Charm
	if err := d.storeCharm(ctx, charmURL, downloadedCharm, tmpFile.Name()); err != nil {
		return corecharm.Origin{}, errors.Annotatef(err, "storing charm %q from origin %v", charmURL, requestedOrigin)
	}

	return actualOrigin, nil
}

func (d *Downloader) downloadAndHash(charmURL *charm.URL, requestedOrigin corecharm.Origin, repo CharmRepository, dstPath string) (DownloadedCharm, corecharm.Origin, error) {
	d.logger.Debugf("downloading charm %q from requested origin %v", charmURL, requestedOrigin)
	chArchive, actualOrigin, err := repo.DownloadCharm(charmURL, requestedOrigin, dstPath)
	if err != nil {
		return DownloadedCharm{}, corecharm.Origin{}, errors.Trace(err)
	}
	d.logger.Debugf("downloaded charm %q from actual origin %v", charmURL, actualOrigin)

	// Calculate SHA256 for the downloaded archive
	f, err := os.Open(dstPath)
	if err != nil {
		return DownloadedCharm{}, corecharm.Origin{}, errors.Annotatef(err, "cannot read downloaded charm")
	}
	defer func() { _ = f.Close() }()

	sha, size, err := utils.ReadSHA256(f)
	if err != nil {
		return DownloadedCharm{}, corecharm.Origin{}, errors.Annotate(err, "cannot calculate SHA256 hash of charm")
	}

	d.logger.Tracef("downloadResult(%q) sha: %q, size: %d", f.Name(), sha, size)
	return DownloadedCharm{
		Charm:        chArchive,
		CharmVersion: chArchive.Version(),
		Size:         size,
		LXDProfile:   chArchive.LXDProfile(),
		SHA256:       sha,
	}, actualOrigin, nil
}

func (d *Downloader) storeCharm(ctx context.Context, charmURL *charm.URL, dc DownloadedCharm, archivePath string) error {
	charmArchive, err := os.Open(archivePath)
	if err != nil {
		return errors.Annotatef(err, "unable to open downloaded charm archive at %q", archivePath)
	}
	defer func() { _ = charmArchive.Close() }()

	dc.CharmData = charmArchive
	if err := d.storage.Store(ctx, charmURL.String(), dc); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (d *Downloader) normalizePlatform(charmURL string, platform corecharm.Platform) (corecharm.Platform, error) {
	arc := platform.Architecture
	if platform.Architecture == "" || platform.Architecture == "all" {
		d.logger.Warningf("received charm Architecture: %q, changing to %q, for charm %q", platform.Architecture, arch.DefaultArchitecture, charmURL)
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

func (d *Downloader) getRepo(src corecharm.Source) (CharmRepository, error) {
	repo, err := d.repoGetter.GetCharmRepository(src)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return repo, nil
}

// lxdProfiler is an adaptor that allows passing a charm.LXDProfile to the
// core/lxdprofile validation logic.
type lxdProfiler struct {
	profile *charm.LXDProfile
}

func (p lxdProfiler) LXDProfile() lxdprofile.LXDProfile {
	return p.profile
}
