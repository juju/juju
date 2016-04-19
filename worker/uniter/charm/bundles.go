// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"net/url"
	"os"
	"path"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/downloader"
)

// Download exposes the downloader.Download methods needed here.
type Download interface {
	// Wait blocks until the download completes or the abort channel
	// receives.
	Wait(abort <-chan struct{}) (*os.File, error)
}

// BundlesDir is responsible for storing and retrieving charm bundles
// identified by state charms.
type BundlesDir struct {
	path          string
	startDownload func(downloader.Request) (Download, error)
}

// NewBundlesDir returns a new BundlesDir which uses path for storage.
func NewBundlesDir(path string, startDownload func(downloader.Request) (Download, error)) *BundlesDir {
	if startDownload == nil {
		startDownload = func(req downloader.Request) (Download, error) {
			opener := downloader.NewHTTPBlobOpener(utils.NoVerifySSLHostnames)
			dl := downloader.StartDownload(req, opener)
			return dl, nil
		}
	}

	return &BundlesDir{
		path:          path,
		startDownload: startDownload,
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
	archiveURLs, err := info.ArchiveURLs()
	if err != nil {
		return errors.Annotatef(err, "failed to get download URLs for charm %q", info.URL())
	}

	dir := d.downloadsPath()
	filename, err := download(info, archiveURLs, dir, d.startDownload, abort)
	if err != nil {
		return errors.Annotatef(err, "failed to download charm %q from %q", info.URL(), archiveURLs)
	}
	defer errors.DeferredAnnotatef(&err, "downloaded but failed to copy charm to %q from %q", target, filename)

	if err := os.MkdirAll(d.path, 0755); err != nil {
		return errors.Trace(err)
	}
	// We must make sure that the file is closed by this point since
	// renaming an open file is not possible on Windows
	if err := os.Rename(filename, target); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func download(info BundleInfo, archiveURLs []*url.URL, targetDir string, startDownload func(downloader.Request) (Download, error), abort <-chan struct{}) (filename string, err error) {
	verify := downloader.NewSha256Verifier(info.ArchiveSha256)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", errors.Trace(err)
	}

	for _, archiveURL := range archiveURLs {
		logger.Infof("downloading %s from %s", info.URL(), archiveURL)
		dl, err2 := startDownload(downloader.Request{
			URL:       archiveURL,
			TargetDir: targetDir,
			Verify:    verify,
		})
		if err2 != nil {
			return "", errors.Trace(err2)
		}
		file, err2 := dl.Wait(abort)
		err = err2
		if errors.IsNotValid(err2) {
			logger.Errorf("download from %s invalid: %v", archiveURL, err2)
			break
		}
		if err == nil {
			defer file.Close()
			filename = file.Name()
			break
		}
		logger.Errorf("download request to %s failed: %v", archiveURL, err)
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	return filename, nil
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

// downloadsPath returns the path to the directory into which charms are
// downloaded.
func (d *BundlesDir) downloadsPath() string {
	return path.Join(d.path, "downloads")
}
