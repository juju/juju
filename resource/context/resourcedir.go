// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/juju/resource"
	"github.com/juju/utils"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	goyaml "gopkg.in/yaml.v2"
)

type resourceDirectorySpec struct {
	name    string
	dirname string
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

// TODO(ericsnow) Need a lockfile around create/write?

type resourceDirectory struct {
	resourceDirectorySpec

	res *resource.Resource
}

func newResourceDirectory(spec resourceDirectorySpec) *resourceDirectory {
	rd := &resourceDirectory{
		resourceDirectorySpec: spec,
	}
	return rd
}

func (rd resourceDirectory) move(dataDir string) *resourceDirectory {
	return &resourceDirectory{
		resourceDirectorySpec: newResourceDirectorySpec(dataDir, rd.name),
		res: rd.res,
	}
}

func (rd *resourceDirectory) readInfo(open func(string) (io.ReadCloser, error)) (resource.Resource, error) {
	if rd.res != nil {
		return *rd.res, nil
	}
	var res resource.Resource

	file, err := open(rd.infoPath())
	if err != nil {
		if os.IsNotExist(err) {
			return res, errors.NotFoundf(".info.yaml for %q", rd.name)
		}
		return res, errors.Trace(err)
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		return res, errors.Trace(err)
	}

	if err := goyaml.Unmarshal(data, &res); err != nil {
		return res, errors.Trace(err)
	}

	rd.res = &res
	return res, nil
}

func (rd *resourceDirectory) writeInfo(res resource.Resource, create func(string) (io.WriteCloser, error)) error {
	data, err := goyaml.Marshal(res)
	if err != nil {
		return errors.Trace(err)
	}

	file, err := create(rd.infoPath())
	if err != nil {
		if os.IsNotExist(err) {
			// TODO(ericsnow) Create the directory?
			return errors.NotFoundf("resource directory for %q", rd.name)
		}
		return errors.Trace(err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		// TODO(ericsnow) Ensure the file is deleted?
		return errors.Trace(err)
	}

	rd.res = &res
	return nil
}

func (rd *resourceDirectory) openResource(relPath []string, open func(string) (io.ReadCloser, error)) (io.ReadCloser, error) {
	if len(relPath) == 0 {
		res, err := rd.readInfo(open)
		if err != nil {
			return nil, errors.Trace(err)
		}
		relPath = []string{res.Path}
	}
	filename := rd.resolve(relPath...)

	reader, err := open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFoundf("resource file %q", filename)
		}
		return nil, errors.Trace(err)
	}

	return reader, nil
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
