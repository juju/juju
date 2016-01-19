// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/juju/resource"
	"github.com/juju/utils"
	"github.com/juju/utils/hash"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

// HookContextFacade is the name of the API facade for resources in the uniter.
const HookContextFacade = resource.ComponentName + "-hook-context"

// APIClient exposes the uniter API functionality needed for resources.
type APIClient interface {
	// GetResource returns the resource info and content for the given
	// name (and unit-implied service).
	GetResource(resourceName string) (resource.Resource, io.ReadCloser, error)
}

// HookContext exposes the functionality exposed by the resources context.
type HookContext interface {
	// DownloadResource downloads the named resource and returns
	// the path to which it was downloaded.
	DownloadResource(name string) (filePath string, _ error)
}

// NewContextAPI returns a new Content for the given API client and data dir.
func NewContextAPI(apiClient APIClient, dataDir string) *Context {
	return &Context{
		apiClient: apiClient,
		dataDir:   dataDir,
	}
}

// Content is the resources portion of a uniter hook context.
type Context struct {
	apiClient APIClient
	dataDir   string
}

// DownloadResource downloads the named resource and returns the path
// to which it was downloaded. If the resource does not exist or has
// not been uploaded yet then errors.NotFound is returned.
//
// Note that the downloaded file is checked for correctness.
func (c *Context) DownloadResource(name string) (string, error) {
	// TODO(katco): Check to see if we have latest version

	checksumWriter := charmresource.NewFingerprintHash()

	resourceInfo, resourceReader, err := c.apiClient.GetResource(name)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer resourceReader.Close()

	hashingReader := hash.NewHashingReader(resourceReader, checksumWriter)
	sizingReader := utils.NewSizingReader(hashingReader)

	resourcePath := resolveResourcePath(c.unitDataDirPath, resourceInfo)
	if err := downloadAndWriteResourceToDisk(resourcePath, sizingReader); err != nil {
		return "", errors.Trace(err)
	}

	size := sizingReader.Size()
	if size != resourceInfo.Size {
		return "", errors.Errorf("resource size does not match expected (%d != %d)", size, resourceInfo.Size)
	}
	fp := checksumWriter.Fingerprint()
	if bytes.Equal(fp.Bytes(), resourceInfo.Fingerprint.Bytes()) == false {
		return "", errors.Errorf("resource fingerprint does not match expected (%q != %q)", fp, resourceInfo.Fingerprint)
	}

	return resourcePath, nil
}

// Flush implements jujuc.Context.
func (c *Context) Flush() error {
	return nil
}

// resolveResourcePath returns the full path to the resource.
func resolveResourcePath(unitPath string, resourceInfo resource.Resource) string {
	return filepath.Join(unitPath, resourceInfo.Name, resourceInfo.Path)
}

// downloadAndWriteResourceToDisk stores the provided resource content
// to disk at the given path.
func downloadAndWriteResourceToDisk(resourcePath string, resourceReader io.Reader) error {
	// TODO(ericsnow) This needs to be atomic?
	// (e.g. write to separate dir and move the dir into place)
	// TODO(katco): Potential race-condition: two commands running at
	// once.
	if err := os.MkdirAll(filepath.Dir(resourcePath), 0755); err != nil {
		return errors.Annotate(err, "could not create resource dir")
	}
	resourceHandle, err := os.Create(resourcePath)
	if err != nil {
		return errors.Annotate(err, "could not create new file for resource")
	}

	if _, err := io.Copy(resourceHandle, resourceReader); err != nil {
		return errors.Annotate(err, "could not write resource to file")
	}

	return nil
}
