// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"io"
	"os"
	"path/filepath"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/context/internal"
)

var logger = loggo.GetLogger("juju.resource.context")

// HookContextFacade is the name of the API facade for resources in the uniter.
const HookContextFacade = "ResourcesHookContext"

// APIClient exposes the uniter API functionality needed for resources.
type APIClient interface {
	// GetResource returns the resource info and content for the given
	// name (and unit-implied application).
	GetResource(resourceName string) (resource.Resource, io.ReadCloser, error)
}

// Content is the resources portion of a uniter hook context.
type Context struct {
	apiClient APIClient

	// dataDir is the path to the directory where all resources are
	// stored for a unit. It will look something like this:
	//
	//   /var/lib/juju/agents/unit-spam-1/resources
	dataDir string
}

// NewContextAPI returns a new Content for the given API client and data dir.
func NewContextAPI(apiClient APIClient, dataDir string) *Context {
	return &Context{
		apiClient: apiClient,
		dataDir:   dataDir,
	}
}

// Flush implements hooks.Context.
func (c *Context) Flush() error {
	return nil
}

// Download downloads the named resource and returns the path
// to which it was downloaded. If the resource does not exist or has
// not been uploaded yet then errors.NotFound is returned.
//
// Note that the downloaded file is checked for correctness.
func (c *Context) Download(name string) (string, error) {
	deps := &contextDeps{
		APIClient: c.apiClient,
		name:      name,
		dataDir:   c.dataDir,
	}
	path, err := internal.ContextDownload(deps)
	if err != nil {
		return "", errors.Trace(err)
	}
	return path, nil
}

// contextDeps implements all the external dependencies
// of ContextDownload().
type contextDeps struct {
	APIClient
	name    string
	dataDir string
}

func (deps *contextDeps) NewContextDirectorySpec() internal.ContextDirectorySpec {
	return internal.NewContextDirectorySpec(deps.dataDir, deps.name, deps)
}

func (deps *contextDeps) OpenResource() (internal.ContextOpenedResource, error) {
	return internal.OpenResource(deps.name, deps)
}

func (deps *contextDeps) Download(target internal.DownloadTarget, remote internal.ContextOpenedResource) error {
	return internal.Download(target, remote)
}

func (deps *contextDeps) WriteContent(target io.Writer, content internal.Content) error {
	return internal.WriteContent(target, content, deps)
}

func (deps contextDeps) CloseAndLog(closer io.Closer, label string) {
	internal.CloseAndLog(closer, label, logger)
}

func (deps contextDeps) MkdirAll(dirname string) error {
	return os.MkdirAll(dirname, 0755)
}

func (deps contextDeps) CreateWriter(filename string) (io.WriteCloser, error) {
	// TODO(ericsnow) chmod 0644?
	return os.Create(filename)
}

func (deps contextDeps) RemoveDir(dirname string) error {
	return os.RemoveAll(dirname)
}

func (deps contextDeps) Copy(target io.Writer, source io.Reader) error {
	_, err := io.Copy(target, source)
	return err
}

func (deps contextDeps) FingerprintMatches(filename string, expected charmresource.Fingerprint) (bool, error) {
	return FingerprintMatcher{}.FingerprintMatches(filename, expected)
}

func (deps contextDeps) Join(path ...string) string {
	return filepath.Join(path...)
}

func (deps contextDeps) NewChecker(content internal.Content) internal.ContentChecker {
	var sizer utils.SizeTracker
	checksumWriter := charmresource.NewFingerprintHash()
	return internal.NewContentChecker(content, &sizer, checksumWriter)
}
