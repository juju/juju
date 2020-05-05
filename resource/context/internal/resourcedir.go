// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// TODO(ericsnow) Move this file elsewhere?
//  (e.g. top-level resource pkg, charm/resource)

import (
	"io"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
)

// DirectorySpec identifies information for a resource directory.
type DirectorySpec struct {
	// Name is the resource name.
	Name string

	// Dirname is the path to the resource directory.
	Dirname string

	// Deps is the external dependencies of DirectorySpec.
	Deps DirectorySpecDeps
}

// NewDirectorySpec returns a new directory spec for the given info.
func NewDirectorySpec(dataDir, name string, deps DirectorySpecDeps) *DirectorySpec {
	dirname := deps.Join(dataDir, name)

	spec := &DirectorySpec{
		Name:    name,
		Dirname: dirname,

		Deps: deps,
	}
	return spec
}

// Resolve returns the fully resolved file path, relative to the directory.
func (spec DirectorySpec) Resolve(path ...string) string {
	return spec.Deps.Join(append([]string{spec.Dirname}, path...)...)
}

// TODO(ericsnow) Make IsUpToDate a stand-alone function?

// IsUpToDate determines whether or not the content matches the resource directory.
func (spec DirectorySpec) IsUpToDate(content Content) (bool, error) {
	filename := spec.Resolve(spec.Name)
	ok, err := spec.Deps.FingerprintMatches(filename, content.Fingerprint)
	return ok, errors.Trace(err)
}

// Initialize preps the spec'ed directory and returns it.
func (spec DirectorySpec) Initialize() (*Directory, error) {
	if err := spec.Deps.MkdirAll(spec.Dirname); err != nil {
		return nil, errors.Annotate(err, "could not create resource dir")
	}

	return NewDirectory(&spec, spec.Deps), nil
}

// DirectorySpecDeps exposes the external depenedencies of DirectorySpec.
type DirectorySpecDeps interface {
	DirectoryDeps

	// FingerprintMatches determines whether or not the identified file
	// exists and has the provided fingerprint.
	FingerprintMatches(filename string, fp charmresource.Fingerprint) (bool, error)

	// Join exposes the functionality of filepath.Join().
	Join(...string) string

	// MkdirAll exposes the functionality of os.MkdirAll().
	MkdirAll(string) error
}

// TempDirectorySpec represents a resource directory placed under a temporary data dir.
type TempDirectorySpec struct {
	*DirectorySpec

	// CleanUp cleans up the temp directory in which the resource
	// directory is placed.
	CleanUp func() error
}

// NewTempDirectorySpec creates a new temp directory spec
// for the given resource.
func NewTempDirectorySpec(name string, deps TempDirDeps) (*TempDirectorySpec, error) {
	tempDir, err := deps.NewTempDir()
	if err != nil {
		return nil, errors.Trace(err)
	}

	spec := &TempDirectorySpec{
		DirectorySpec: NewDirectorySpec(tempDir, name, deps),
		CleanUp: func() error {
			return deps.RemoveDir(tempDir)
		},
	}
	return spec, nil
}

// TempDirDeps exposes the external functionality needed by
// NewTempDirectorySpec().
type TempDirDeps interface {
	DirectorySpecDeps

	// NewTempDir returns the path to a new temporary directory.
	NewTempDir() (string, error)

	// RemoveDir deletes the specified directory.
	RemoveDir(string) error
}

// Close implements io.Closer.
func (spec TempDirectorySpec) Close() error {
	if err := spec.CleanUp(); err != nil {
		return errors.Annotate(err, "could not clean up temp dir")
	}
	return nil
}

// Directory represents a resource directory.
type Directory struct {
	*DirectorySpec

	// Deps holds the external dependencies of the directory.
	Deps DirectoryDeps
}

// NewDirectory returns a new directory for the provided spec.
func NewDirectory(spec *DirectorySpec, deps DirectoryDeps) *Directory {
	dir := &Directory{
		DirectorySpec: spec,
		Deps:          deps,
	}
	return dir
}

// Write writes all relevant files from the given source
// to the directory.
func (dir *Directory) Write(opened ContentSource) error {
	// TODO(ericsnow) Also write the info file...

	relPath := opened.Info().Path
	if err := dir.WriteContent(relPath, opened.Content()); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// WriteContent writes the resource file to the given path
// within the directory.
func (dir *Directory) WriteContent(relPath string, content Content) error {
	if len(relPath) == 0 {
		// TODO(ericsnow) Use rd.readInfo().Path, like openResource() does?
		return errors.NotImplementedf("")
	}
	filename := dir.Resolve(relPath)

	target, err := dir.Deps.CreateWriter(filename)
	if err != nil {
		return errors.Annotate(err, "could not create new file for resource")
	}
	defer dir.Deps.CloseAndLog(target, filename)

	if err := dir.Deps.WriteContent(target, content); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// DirectoryDeps exposes the external functionality needed by Directory.
type DirectoryDeps interface {
	// CreateWriter creates a new writer to which the resource file
	// will be written.
	CreateWriter(string) (io.WriteCloser, error)

	// CloseAndLog closes the closer and logs any error.
	CloseAndLog(io.Closer, string)

	// WriteContent writes the content to the directory.
	WriteContent(io.Writer, Content) error
}
