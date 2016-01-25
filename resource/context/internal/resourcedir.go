// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// TODO(ericsnow) Move this file elsewhere?
//  (e.g. top-level resource pkg, charm/resource)

import (
	"io"

	"github.com/juju/errors"
)

type DirectorySpec struct {
	Name    string
	Dirname string

	deps DirectorySpecDeps
}

func NewDirectorySpec(dataDir, name string, deps DirectorySpecDeps) *DirectorySpec {
	dirname := deps.Join(dataDir, name)

	spec := &DirectorySpec{
		Name:    name,
		Dirname: dirname,

		deps: deps,
	}
	return spec
}

func (spec DirectorySpec) Resolve(path ...string) string {
	return spec.deps.Join(append([]string{spec.Dirname}, path...)...)
}

func (spec DirectorySpec) infoPath() string {
	return spec.Resolve(".info.yaml")
}

func (spec DirectorySpec) IsUpToDate(content Content) (bool, error) {
	// TODO(katco): Check to see if we have latest version
	return false, nil
}

//func (spec DirectorySpec) exists(stat func(string) (os.FileInfo, error)) (bool, error) {
//	fileinfo, err := stat(spec.Dirname)
//	if err != nil {
//		if os.IsNotExist(err) {
//			return false, nil
//		}
//		return false, errors.Trace(err)
//	}
//	if !fileinfo.IsDir() {
//		return false, errors.NotFoundf("%q not a directory", spec.Dirname)
//	}
//	return true, nil
//}

func (spec DirectorySpec) Open() (*Directory, error) {
	if err := spec.deps.MkdirAll(spec.Dirname); err != nil {
		return nil, errors.Annotate(err, "could not create resource dir")
	}

	return NewDirectory(&spec, spec.deps), nil
}

type DirectorySpecDeps interface {
	DirectoryDeps

	Join(...string) string

	MkdirAll(string) error
}

type TempDirectorySpec struct {
	*DirectorySpec

	CleanUp func() error
}

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

type TempDirDeps interface {
	DirectorySpecDeps

	NewTempDir() (string, error)

	RemoveDir(string) error
}

func (spec TempDirectorySpec) Close() error {
	if err := spec.CleanUp(); err != nil {
		return errors.Annotate(err, "could not clean up temp dir")
	}
	return nil
}

type Directory struct {
	*DirectorySpec

	deps DirectoryDeps
}

func NewDirectory(spec *DirectorySpec, deps DirectoryDeps) *Directory {
	dir := &Directory{
		DirectorySpec: spec,

		deps: deps,
	}
	return dir
}

func (dir *Directory) Write(opened ContentSource) error {
	// TODO(ericsnow) Also write the info file...

	relPath := []string{opened.Info().Path}
	if err := dir.WriteContent(relPath, opened.Content()); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (dir *Directory) WriteContent(relPath []string, content Content) error {
	if len(relPath) == 0 {
		// TODO(ericsnow) Use rd.readInfo().Path, like openResource() does?
		return errors.NotImplementedf("")
	}
	filename := dir.Resolve(relPath...)

	err := WriteContent(content, &dirWriteContentDeps{
		DirectoryDeps: dir.deps,
		filename:      filename,
	})
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

type DirectoryDeps interface {
	NewChecker(Content) ContentChecker

	CreateFile(string) (io.WriteCloser, error)
}

type dirWriteContentDeps struct {
	DirectoryDeps
	filename string
}

func (deps dirWriteContentDeps) CreateTarget() (io.WriteCloser, error) {
	return deps.CreateFile(deps.filename)
}
