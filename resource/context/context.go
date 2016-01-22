// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"io"
	"os"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/resource"
)

var logger = loggo.GetLogger("juju.resource.context")

// HookContextFacade is the name of the API facade for resources in the uniter.
const HookContextFacade = resource.ComponentName + "-hook-context"

// APIClient exposes the uniter API functionality needed for resources.
type APIClient interface {
	// GetResource returns the resource info and content for the given
	// name (and unit-implied service).
	GetResource(resourceName string) (resource.Resource, io.ReadCloser, error)
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

// Download downloads the named resource and returns the path
// to which it was downloaded. If the resource does not exist or has
// not been uploaded yet then errors.NotFound is returned.
//
// Note that the downloaded file is checked for correctness.
func (c *Context) Download(name string) (string, error) {
	// TODO(katco): Potential race-condition: two commands running at
	// once. Solve via collision using os.Mkdir() with a uniform
	// temp dir name (e.g. "<datadir>/.<res name>.download")?

	resDirSpec := newResourceDirectorySpec(c.dataDir, name)

	dl, err := c.newDownloader(resDirSpec)
	defer closeAndLog(dl, "downloader") // This ensures that the tempdir is cleaned up.
	if err != nil {
		return "", errors.Trace(err)
	}
	path := resDirSpec.resolve(dl.path())

	tempDir, err := dl.download()
	if err != nil {
		return "", errors.Trace(err)
	}

	if tempDir != nil { // ...otherwise we're up to date already!
		oldDir := tempDir.resolve()
		newDir := resDirSpec.resolve()
		if err := c.replaceDirectory(newDir, oldDir); err != nil {
			return "", errors.Annotate(err, "could not replace existing resource directory")
		}
	}

	return path, nil
}

func (c *Context) newDownloader(spec resourceDirectorySpec) (downloader, error) {
	dl := downloader{
		downloaderDeps: c.downloaderDeps,
	}

	tempDirSpec, err := newTempResourceDir(spec.name, c.tempDirDeps)
	if err != nil {
		return dl, errors.Trace(err)
	}
	dl.dirSpec = tempDirSpec

	remote, err := openResource(spec.name, c.apiClient)
	if err != nil {
		return dl, errors.Trace(err)
	}
	dl.remote = remote

	// TODO(katco): Check to see if we have latest version
	// (and set dl.isUpToDate)

	return dl, nil
}

type contextDeps struct {
	downloaderDeps downloaderDeps
	tempDirDeps    tempDirDeps

	replaceDirectory func(string, string) error
}

func newContextDeps() contextDeps {
	return contextDeps{
		downloaderDeps: newDownloaderDeps(),
		tempDirDeps:    newTempDirDeps(),

		replaceDirectory: replaceDirectory,
	}
}

// replaceDirectory replaces the target directory with the source. This
// involves removing the target if it exists and then moving the source
// into place.
func replaceDirectory(targetDir, sourceDir string) error {
	// TODO(ericsnow) Move it out of the way and remove it after the rename.
	if err := os.RemoveAll(targetDir); err != nil {
		return errors.Trace(err)
	}
	if err := os.Rename(targetDir, sourceDir); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func closeAndLog(closer io.Closer, label string) {
	if err := closer.Close(); err != nil {
		logger.Errorf("while closing %s: %v", label, err)
	}
}
