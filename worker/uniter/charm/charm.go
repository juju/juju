// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"net/url"
	"os"
	"path"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/downloader"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/utils"
)

// BundleReader primarily exists to make BundlesDir mockable.
type BundleReader interface {

	// Read returns the bundle identified by the supplied info. The abort chan
	// can be used to notify an implementation that it need not complete the
	// operation, and can immediately error out if it is convenient to do so.
	Read(bi BundleInfo, abort <-chan struct{}) (*charm.Bundle, error)
}

// BundleInfo holds bundle information for a charm.
type BundleInfo interface {
	URL() *charm.URL
	ArchiveURL() (*url.URL, bool, error)
	ArchiveSha256() (string, error)
}

// BundlesDir is responsible for storing and retrieving charm bundles
// identified by state charms.
type BundlesDir struct {
	path string
}

// NewBundlesDir returns a new BundlesDir which uses path for storage.
func NewBundlesDir(path string) *BundlesDir {
	return &BundlesDir{path}
}

// Read returns a charm bundle from the directory. If no bundle exists yet,
// one will be downloaded and validated and copied into the directory before
// being returned. Downloads will be aborted if a value is received on abort.
func (d *BundlesDir) Read(info BundleInfo, abort <-chan struct{}) (*charm.Bundle, error) {
	path := d.bundlePath(info)
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		} else if err = d.download(info, abort); err != nil {
			return nil, err
		}
	}
	return charm.ReadBundle(path)
}

// download fetches the supplied charm and checks that it has the correct sha256
// hash, then copies it into the directory. If a value is received on abort, the
// download will be stopped.
func (d *BundlesDir) download(info BundleInfo, abort <-chan struct{}) (err error) {
	archiveURL, disableSSLHostnameVerification, err := info.ArchiveURL()
	if err != nil {
		return err
	}
	defer utils.ErrorContextf(&err, "failed to download charm %q from %q", info.URL(), archiveURL)
	dir := d.downloadsPath()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	aurl := archiveURL.String()
	log.Infof("worker/uniter/charm: downloading %s from %s", info.URL(), aurl)
	if disableSSLHostnameVerification {
		log.Infof("worker/uniter/charm: SSL hostname verification disabled")
	}
	dl := downloader.New(aurl, dir, disableSSLHostnameVerification)
	defer dl.Stop()
	for {
		select {
		case <-abort:
			log.Infof("worker/uniter/charm: download aborted")
			return fmt.Errorf("aborted")
		case st := <-dl.Done():
			if st.Err != nil {
				return st.Err
			}
			log.Infof("worker/uniter/charm: download complete")
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
			log.Infof("worker/uniter/charm: download verified")
			if err := os.MkdirAll(d.path, 0755); err != nil {
				return err
			}
			return os.Rename(st.File.Name(), d.bundlePath(info))
		}
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
