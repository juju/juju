// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"net/url"
	"os"
	"path"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/downloader"
)

// Download exposes the downloader.Download methods needed here.
type Downloader interface {
	// Download starts a new charm archive download, waits for it to
	// complete, and returns the local name of the file.
	Download(req downloader.Request) (string, error)
}

// BundlesDir is responsible for storing and retrieving charm bundles
// identified by state charms.
type BundlesDir struct {
	path       string
	downloader Downloader
	logger     Logger
}

// NewBundlesDir returns a new BundlesDir which uses path for storage.
func NewBundlesDir(path string, dlr Downloader, logger Logger) *BundlesDir {
	if dlr == nil {
		dlr = downloader.New(downloader.NewArgs{
			HostnameVerification: utils.NoVerifySSLHostnames,
		})
	}
	return &BundlesDir{
		path:       path,
		downloader: dlr,
		logger:     logger,
	}
}

// Read returns a charm bundle from the directory. If no bundle exists yet,
// one will be downloaded and validated and copied into the directory before
// being returned. Downloads will be aborted if a value is received on abort.
func (d *BundlesDir) Read(info BundleInfo, abort <-chan struct{}) (Bundle, error) {
	path := d.bundlePath(info)
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err := d.download(info, path, abort); err != nil {
			return nil, err
		}
	}
	return charm.ReadCharmArchive(path)
}

// download fetches the supplied charm and checks that it has the correct sha256
// hash, then copies it into the directory. If a value is received on abort, the
// download will be stopped.
func (d *BundlesDir) download(info BundleInfo, target string, abort <-chan struct{}) (err error) {
	// First download...
	curl, err := url.Parse(info.URL().String())
	if err != nil {
		return errors.Annotate(err, "could not parse charm URL")
	}
	expectedSha256, err := info.ArchiveSha256()
	req := downloader.Request{
		URL:       curl,
		TargetDir: downloadsPath(d.path),
		Verify:    downloader.NewSha256Verifier(expectedSha256),
		Abort:     abort,
	}
	d.logger.Infof("downloading %s from API server", info.URL())
	filename, err := d.downloader.Download(req)
	if err != nil {
		return errors.Annotatef(err, "failed to download charm %q from API server", info.URL())
	}
	defer errors.DeferredAnnotatef(&err, "downloaded but failed to copy charm to %q from %q", target, filename)

	// ...then move the right location.
	if err := os.MkdirAll(d.path, 0755); err != nil {
		return errors.Trace(err)
	}
	if err := os.Rename(filename, target); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// bundlePath returns the path to the location where the verified charm
// bundle identified by info will be, or has been, saved.
func (d *BundlesDir) bundlePath(info BundleInfo) string {
	return d.bundleURLPath(info.URL())
}

// bundleURLPath returns the path to the location where the verified charm
// bundle identified by url will be, or has been, saved.
func (d *BundlesDir) bundleURLPath(url *charm.URL) string {
	return path.Join(d.path, charm.Quote(url.String()))
}

// ClearDownloads removes any entries in the temporary bundle download
// directory. It is intended to be called on uniter startup.
func ClearDownloads(bundlesDir string) error {
	downloadDir := downloadsPath(bundlesDir)
	err := os.RemoveAll(downloadDir)
	return errors.Annotate(err, "unable to clear bundle downloads")
}

// downloadsPath returns the path to the directory into which charms are
// downloaded.
func downloadsPath(bunsDir string) string {
	return path.Join(bunsDir, "downloads")
}
