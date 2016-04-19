// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
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
	// Stop stops any download that's in progress.
	Stop()

	// Done returns a channel that receives a status when the download
	// has completed. It is the receiver's responsibility to close and
	// remove the received file.
	Done() <-chan downloader.Status
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
			dl := downloader.New(req, utils.NoVerifySSLHostnames)
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
		} else if err = d.download(info, abort); err != nil {
			return nil, err
		}
	}
	return charm.ReadCharmArchive(path)
}

// download fetches the supplied charm and checks that it has the correct sha256
// hash, then copies it into the directory. If a value is received on abort, the
// download will be stopped.
func (d *BundlesDir) download(info BundleInfo, abort <-chan struct{}) (err error) {
	archiveURLs, err := info.ArchiveURLs()
	if err != nil {
		return errors.Annotatef(err, "failed to get download URLs for charm %q", info.URL())
	}
	defer errors.DeferredAnnotatef(&err, "failed to download charm %q from %q", info.URL(), archiveURLs)
	dir := d.downloadsPath()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	var st downloader.Status
	for _, archiveURL := range archiveURLs {
		logger.Infof("downloading %s from %s", info.URL(), archiveURL)
		st, err = tryDownload(archiveURL, dir, d.startDownload, abort)
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	logger.Infof("download complete")
	defer st.File.Close()
	actualSha256, _, err := utils.ReadSHA256(st.File)
	if err != nil {
		return err
	}
	archiveSha256, err := info.ArchiveSha256()
	if err != nil {
		return err
	}
	if actualSha256 != archiveSha256 {
		return fmt.Errorf(
			"expected sha256 %q, got %q", archiveSha256, actualSha256,
		)
	}
	logger.Infof("download verified")
	if err := os.MkdirAll(d.path, 0755); err != nil {
		return err
	}
	// Renaming an open file is not possible on Windows
	st.File.Close()
	return os.Rename(st.File.Name(), d.bundlePath(info))
}

func tryDownload(url *url.URL, dir string, startDownload func(downloader.Request) (Download, error), abort <-chan struct{}) (downloader.Status, error) {
	dl, err := startDownload(downloader.Request{
		URL:       url,
		TargetDir: dir,
	})
	if err != nil {
		return downloader.Status{}, errors.Trace(err)
	}
	defer dl.Stop()

	select {
	case <-abort:
		logger.Infof("download aborted")
		return downloader.Status{}, errors.New("aborted")
	case st := <-dl.Done():
		if st.Err != nil {
			return downloader.Status{}, st.Err
		}
		return st, nil
	}
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
