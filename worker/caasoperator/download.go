// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"net/url"
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/downloader"
)

// Downloader provides an interface for downloading files to disk.
type Downloader interface {
	// Download downloads a file to a local directory, and
	// returns the local path to the file.
	Download(downloader.Request) (string, error)
}

func downloadCharm(
	dl Downloader,
	curl *charm.URL,
	sha256 string,
	charmDir string,
	abort <-chan struct{},
) error {
	curlURL, err := url.Parse(curl.String())
	if err != nil {
		return errors.Trace(err)
	}

	charmDirDownload := charmDir + ".dl"
	if err := os.RemoveAll(charmDirDownload); err != nil {
		return errors.Trace(err)
	}
	if err := os.MkdirAll(charmDirDownload, 0755); err != nil {
		return errors.Trace(err)
	}
	defer os.RemoveAll(charmDirDownload)

	verify := func(f *os.File) error {
		digest, _, err := utils.ReadSHA256(f)
		if err != nil {
			return errors.Trace(err)
		}
		if digest == sha256 {
			return nil
		}
		return errors.New("SHA256 hash mismatch")
	}

	logger.Debugf("downloading %q to %s...", curlURL, charmDirDownload)
	charmArchivePath, err := dl.Download(downloader.Request{
		URL:       curlURL,
		TargetDir: charmDirDownload,
		Verify:    verify,
		Abort:     abort,
	})
	if err != nil {
		return errors.Trace(err)
	}

	// Unpack the archive to a temporary directory,
	// and then atomically rename that to the target
	// directory.
	charmDirUnpack := charmDir + ".tmp"
	logger.Debugf("unpacking charm into %s...", charmDirUnpack)
	if err := os.RemoveAll(charmDirUnpack); err != nil {
		return errors.Trace(err)
	}
	if err := os.MkdirAll(charmDirUnpack, 0755); err != nil {
		return errors.Trace(err)
	}
	if err := unpackCharm(charmArchivePath, charmDirUnpack); err != nil {
		os.RemoveAll(charmDirUnpack)
		return errors.Trace(err)
	}
	if err := os.Rename(charmDirUnpack, charmDir); err != nil {
		os.RemoveAll(charmDirUnpack)
		return errors.Trace(err)
	}
	logger.Debugf("charm is ready at %s", charmDir)
	return nil
}

func unpackCharm(charmArchivePath, dir string) error {
	charmArchive, err := charm.ReadCharmArchive(charmArchivePath)
	if err != nil {
		return errors.Trace(err)
	}
	return charmArchive.ExpandTo(dir)
}
