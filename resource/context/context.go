// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/juju/resource"
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

// Content is the resources portion of a uniter hook context.
type Context struct {
	contextDeps

	apiClient APIClient
	dataDir   string
}

// NewContextAPI returns a new Content for the given API client and data dir.
func NewContextAPI(apiClient APIClient, dataDir string) *Context {
	return &Context{
		contextDeps: newContextDeps(),

		apiClient: apiClient,
		dataDir:   dataDir,
	}
}

// Flush implements jujuc.Context.
func (c *Context) Flush() error {
	return nil
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

	//resDir := c.resDir(name)
	//current := c.read

	tempDir, err := c.tempDir()
	if err != nil {
		return "", errors.Trace(err)
	}
	defer c.removeDir(tempDir)

	spec := newResourceDirectorySpec(tempDir, name)
	resDir, err := spec.open(c.mkdirAll)
	if err != nil {
		return "", errors.Trace(err)
	}

	info, err := c.downloadResource(name, resDir)
	if err != nil {
		return "", errors.Trace(err)
	}

	resDir, err = c.replaceResourceDir(resDir)
	if err != nil {
		return "", errors.Trace(err)
	}

	resourcePath := resDir.resolve(info.Path)
	return resourcePath, nil
}

// downloadResource downloads the named resource to the provided path.
func (c *Context) downloadResource(name string, resDir *resourceDirectory) (resource.Resource, error) {
	info, resourceReader, err := c.apiClient.GetResource(name)
	if err != nil {
		return resource.Resource{}, errors.Trace(err)
	}
	defer resourceReader.Close()

	if err := resDir.writeInfo(info, c.createFile); err != nil {
		return resource.Resource{}, errors.Trace(err)
	}

	content := resourceContent{
		data:        resourceReader,
		size:        info.Size,
		fingerprint: info.Fingerprint,
	}
	relPath := []string{info.Path}
	if err := resDir.writeResource(relPath, content, c.createFile); err != nil {
		return resource.Resource{}, errors.Trace(err)
	}

	return info, nil
}

// replaceResourceDir replaces the original resource dir
// with the temporary resource dir.
func (c *Context) replaceResourceDir(oldResDir *resourceDirectory) (*resourceDirectory, error) {
	oldDir := oldResDir.resolve()
	newResDir := oldResDir.move(c.dataDir)
	newDir := newResDir.resolve()

	if err := c.replaceDirectory(newDir, oldDir); err != nil {
		return nil, errors.Annotate(err, "could not replace existing resource directory")
	}

	return newResDir, nil
}

type contextDeps struct {
	tempDir          func() (string, error)
	mkdirAll         func(string) error
	removeDir        func(string) error
	replaceDirectory func(string, string) error
	createFile       func(string) (io.WriteCloser, error)
}

func newContextDeps() contextDeps {
	return contextDeps{
		tempDir:          func() (string, error) { return ioutil.TempDir("", "juju-resource-") },
		mkdirAll:         func(path string) error { return os.MkdirAll(path, 0755) },
		removeDir:        os.RemoveAll,
		replaceDirectory: replaceDirectory,
		createFile:       func(path string) (io.WriteCloser, error) { return os.Create(path) },
	}
}

// replaceDirectory replaces the target directory with the source. This
// involves removing the target if it exists and then moving the source
// into place.
func replaceDirectory(targetDir, sourceDir string) error {
	if err := os.RemoveAll(targetDir); err != nil {
		return errors.Trace(err)
	}
	if err := os.Rename(targetDir, sourceDir); err != nil {
		return errors.Trace(err)
	}
	return nil
}
