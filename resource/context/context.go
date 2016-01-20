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

		createResourceFile: createResourceFile,
		writeResource:      writeResource,
	}
}

// Content is the resources portion of a uniter hook context.
type Context struct {
	apiClient APIClient
	dataDir   string

	createResourceFile func(path string) (io.WriteCloser, error)
	writeResource      func(io.Writer, io.Reader) (int64, charmresource.Fingerprint, error)
}

// DownloadResource downloads the named resource and returns the path
// to which it was downloaded. If the resource does not exist or has
// not been uploaded yet then errors.NotFound is returned.
//
// Note that the downloaded file is checked for correctness.
func (c *Context) DownloadResource(name string) (string, error) {
	// TODO(katco): Potential race-condition: two commands running at
	// once.
	// TODO(katco): Check to see if we have latest version

	resourcePath, _, err := c.downloadResource(name)
	if err != nil {
		return "", errors.Trace(err)
	}

	return resourcePath, nil
}

// Flush implements jujuc.Context.
func (c *Context) Flush() error {
	return nil
}

// downloadResource downloads the named resource to the provided path.
func (c *Context) downloadResource(name string) (string, resource.Resource, error) {
	// TODO(ericsnow) This needs to be atomic?
	// (e.g. write to separate dir and move the dir into place)

	info, resourceReader, err := c.apiClient.GetResource(name)
	if err != nil {
		return "", resource.Resource{}, errors.Trace(err)
	}
	defer resourceReader.Close()

	resourcePath := resolveResourcePath(c.dataDir, info)

	target, err := c.createResourceFile(resourcePath)
	if err != nil {
		return "", resource.Resource{}, errors.Trace(err)
	}
	defer target.Close()

	content := resourceContent{
		data:        resourceReader,
		size:        info.Size,
		fingerprint: info.Fingerprint,
	}
	size, fp, err := c.writeResource(target, content.data)
	if err != nil {
		return "", resource.Resource{}, errors.Trace(err)
	}

	if err := content.verify(size, fp); err != nil {
		return "", resource.Resource{}, errors.Trace(err)
	}

	return resourcePath, info, nil
}

// resourceContent holds a reader for the content of a resource along
// with details about that content.
type resourceContent struct {
	data        io.Reader
	size        int64
	fingerprint charmresource.Fingerprint
}

// verify ensures that the actual resource content details match
// the expected ones.
func (c resourceContent) verify(size int64, fp charmresource.Fingerprint) error {
	if size != c.size {
		return errors.Errorf("resource size does not match expected (%d != %d)", size, c.size)
	}
	if !bytes.Equal(fp.Bytes(), c.fingerprint.Bytes()) {
		return errors.Errorf("resource fingerprint does not match expected (%q != %q)", fp, c.fingerprint)
	}
	return nil
}

// resolveResourcePath returns the full path to the resource.
func resolveResourcePath(unitPath string, resourceInfo resource.Resource) string {
	return filepath.Join(unitPath, resourceInfo.Name, resourceInfo.Path)
}

// createResourceFile creates the file into which a resource's content
// should be written.
func createResourceFile(path string) (io.WriteCloser, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, errors.Annotate(err, "could not create resource dir")
	}
	target, err := os.Create(path)
	if err != nil {
		return nil, errors.Annotate(err, "could not create new file for resource")
	}
	// TODO(ericsnow) chmod 0644?
	return target, nil
}

// writeResource writes the provided (source) resource content
// to the target. The size and fingerprint are returned.
func writeResource(target io.Writer, source io.Reader) (size int64, fp charmresource.Fingerprint, err error) {
	checksumWriter := charmresource.NewFingerprintHash()
	hashingReader := hash.NewHashingReader(source, checksumWriter)
	sizingReader := utils.NewSizingReader(hashingReader)
	source = sizingReader

	if _, err := io.Copy(target, source); err != nil {
		return size, fp, errors.Annotate(err, "could not write resource to file")
	}

	size = sizingReader.Size()
	fp = checksumWriter.Fingerprint()
	return size, fp, nil
}
