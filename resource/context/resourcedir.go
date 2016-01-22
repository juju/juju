// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

// TODO(ericsnow) Move this file elsewhere?
//  (e.g. top-level resource pkg, charm/resource)

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

type resourceDirectorySpec struct {
	name    string
	dirname string

	cleanup func() error
}

func newResourceDirectorySpec(dataDir, name string) resourceDirectorySpec {
	dirname := filepath.Join(dataDir, name)

	rds := resourceDirectorySpec{
		name:    name,
		dirname: dirname,
	}
	return rds
}

func (rds resourceDirectorySpec) resolve(path ...string) string {
	return filepath.Join(append([]string{rds.dirname}, path...)...)
}

func (rds resourceDirectorySpec) infoPath() string {
	return rds.resolve(".info.yaml")
}

func (rds resourceDirectorySpec) CleanUp() error {
	if rds.cleanup != nil {
		return rds.cleanup()
	}
	return nil
}

func (rds resourceDirectorySpec) exists(stat func(string) (os.FileInfo, error)) (bool, error) {
	fileinfo, err := stat(rds.dirname)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Trace(err)
	}
	if !fileinfo.IsDir() {
		return false, errors.NotFoundf("%q not a directory", rds.dirname)
	}
	return true, nil
}

func (rds resourceDirectorySpec) open(mkdirAll func(string) error) (*resourceDirectory, error) {
	if mkdirAll != nil {
		if err := mkdirAll(rds.dirname); err != nil {
			return nil, errors.Annotate(err, "could not create resource dir")
		}
	}

	return newResourceDirectory(rds), nil
}

type tempDirDeps struct {
	newTempDir func() (string, error)
	removeDir  func(string) error
}

func newTempDirDeps() tempDirDeps {
	return tempDirDeps{
		newTempDir: func() (string, error) { return ioutil.TempDir("", "juju-resource-") },
		removeDir:  os.RemoveAll,
	}
}

//newTempDir func() (string, error), removeDir func(string) error) (resourceDirectorySpec, error) {
func newTempResourceDir(name string, deps tempDirDeps) (resourceDirectorySpec, error) {
	var spec resourceDirectorySpec

	tempDir, err := deps.newTempDir()
	if err != nil {
		return spec, errors.Trace(err)
	}

	spec = newResourceDirectorySpec(tempDir, name)
	spec.cleanup = func() error {
		return deps.removeDir(tempDir)
	}

	return spec, nil
}

// TODO(ericsnow) Need a lockfile around create/write?

type resourceDirectory struct {
	resourceDirectorySpec
}

func newResourceDirectory(spec resourceDirectorySpec) *resourceDirectory {
	rd := &resourceDirectory{
		resourceDirectorySpec: spec,
	}
	return rd
}

func (rd resourceDirectory) writeResource(relPath []string, content resourceContent, create func(string) (io.WriteCloser, error)) error {
	if len(relPath) == 0 {
		// TODO(ericsnow) Use rd.readInfo().Path, like openResource() does?
		return errors.NotImplementedf("")
	}
	filename := rd.resolve(relPath...)

	var st utils.SizeTracker
	var checksumWriter *charmresource.FingerprintHash

	source := content.data
	if !content.fingerprint.IsZero() {
		checksumWriter = charmresource.NewFingerprintHash()
		hashingReader := io.TeeReader(content.data, checksumWriter)
		source = io.TeeReader(hashingReader, &st)
	}

	target, err := create(filename)
	if err != nil {
		return errors.Annotate(err, "could not create new file for resource")
	}
	defer target.Close()
	// TODO(ericsnow) chmod 0644?

	if _, err := io.Copy(target, source); err != nil {
		return errors.Annotate(err, "could not write resource to file")
	}

	if checksumWriter != nil {
		size := st.Size()
		fp := checksumWriter.Fingerprint()
		if err := content.verify(size, fp); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}
