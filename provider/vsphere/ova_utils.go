// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/mutex"
)

// CacheLocker is an interface that may be used for locking
// an OVA cache directory.
type CacheLocker interface {
	// Lock locks the OVA cache directory for concurrent access,
	// and returns a function to release the lock or an error.
	Lock() (func(), error)
}

type mutexCacheLocker struct {
	spec mutex.Spec
}

// NewMutexCacheLocker returns an implemenation of CacheLocker
// based on juju/mutex, using the given mutex.Spec.
func NewMutexCacheLocker(spec mutex.Spec) CacheLocker {
	return mutexCacheLocker{spec: spec}
}

// Lock is part of the CacheLocker interface.
func (l mutexCacheLocker) Lock() (func(), error) {
	releaser, err := mutex.Acquire(l.spec)
	if err != nil {
		return nil, err
	}
	return releaser.Release, nil
}

// downloadOVA downloads and extracts the OVA identified by the
// given image metadata into the specified directory. We store
// the extracted OVA in the directory <basedir>/<series>/<arch>/<sha256>.
// If that directory already exists, we'll use it as-is; otherwise
// we delete all existing subdirectories of <basedir>/<series>/<arch>
// (to remove older images), and then download and extract into
// the target directory.
func downloadOVA(
	basedir, series string,
	img *OvaFileMetadata,
	updateProgress func(string),
) (ovaDir string, ovfPath string, _ error) {
	seriesDir := filepath.Join(basedir, series)
	archDir := filepath.Join(seriesDir, img.Arch)
	ovaDir = filepath.Join(archDir, img.Sha256)
	if _, err := os.Stat(ovaDir); err == nil {
		// The directory exists, which means we have previously
		// successfully downloaded and extracted the OVA. We create
		// the target directory atomically at the end of the process.
		logger.Debugf("OVA file previously extracted to %s", ovaDir)
		ovfPath, err := findOVF(ovaDir)
		if err != nil {
			return "", "", errors.Trace(err)
		}
		return ovaDir, ovfPath, nil
	}

	// Remove any existing <series>/<arch> directories, so we don't
	// accumulate old images.
	if err := os.RemoveAll(archDir); err != nil {
		return "", "", errors.Trace(err)
	}

	// Tacking on a ".tmp" suffix ensures that tempdir:
	//  - is within the same filesystem as ovaDir, makingthe
	//    final rename atomic
	//  - will be removed by a subsequent RemoveAll (above)
	//    in the event of failure
	tempdir := ovaDir + ".tmp"
	if err := os.MkdirAll(tempdir, 0755); err != nil {
		return "", "", errors.Trace(err)
	}
	defer os.RemoveAll(tempdir)

	updateProgress(fmt.Sprintf("downloading %s", img.URL))
	logger.Debugf("downloading OVA file from %s", img.URL)
	resp, err := http.Get(img.URL)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", errors.Errorf("downloading %s (status: %d)", img.URL, resp.StatusCode)
	}

	logger.Debugf("extracting OVA to path: %s", ovaDir)
	hash := sha256.New()
	reader := io.TeeReader(resp.Body, hash)
	ovfFilename, err := extractOVA(tempdir, reader)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	// Read any trailing data so we compute the hash correctly.
	if _, err := io.Copy(ioutil.Discard, reader); err != nil {
		return "", "", errors.Trace(err)
	}
	if img.Sha256 != fmt.Sprintf("%x", hash.Sum(nil)) {
		return "", "", errors.Errorf("hash mismatch")
	}
	if err := os.Rename(tempdir, ovaDir); err != nil {
		return "", "", errors.Trace(err)
	}
	logger.Debugf("OVA extracted successfully to %s", ovaDir)
	return ovaDir, filepath.Join(ovaDir, ovfFilename), nil
}

func extractOVA(dir string, body io.Reader) (string, error) {
	tarBallReader := tar.NewReader(body)
	var ovfFilename string
	for {
		header, err := tarBallReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return "", errors.Trace(err)
		}
		logger.Debugf("writing file %s", header.Name)
		if filepath.Ext(header.Name) == ".ovf" {
			ovfFilename = header.Name
		}
		writer, err := os.Create(filepath.Join(dir, header.Name))
		if err != nil {
			return "", errors.Trace(err)
		}
		_, err = io.Copy(writer, tarBallReader)
		writer.Close() // close whether Copy failed or not
		if err != nil {
			return "", errors.Trace(err)
		}
	}
	if ovfFilename == "" {
		return "", errors.NotFoundf(".ovf file")
	}
	return ovfFilename, nil
}

func findOVF(dirpath string) (string, error) {
	dir, err := os.Open(dirpath)
	if err != nil {
		return "", err
	}
	info, err := dir.Readdir(-1)
	if err != nil {
		return "", err
	}
	for _, info := range info {
		filename := info.Name()
		if filepath.Ext(filename) == ".ovf" {
			return filepath.Join(dirpath, filename), nil
		}
	}
	return "", errors.NotFoundf(".ovf file")
}
