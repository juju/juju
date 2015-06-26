// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/juju/errors"
)

// dirFuncs is used to allow the real directory operations to
// be stubbed out for testing.
type dirFuncs interface {
	mkDirAll(path string, perm os.FileMode) error
	lstat(path string) (fi os.FileInfo, err error)
	fileCount(path string) (int, error)
	calculateSize(path string) (sizeInMib uint64, _ error)
	symlink(oldpath, newpath string) error

	// bindMount remounts the directory "source" at "target",
	// so that the source tree is available in both locations.
	// If "target" already refers to "source" in this manner,
	// then the bindMount operation is a no-op.
	bindMount(source, target string) error

	// mountPoint returns the mount-point that contains the
	// specified path.
	mountPoint(path string) (string, error)

	// mountPointSource returns the source of the mount-point
	// that contains the specified path.
	mountPointSource(path string) (string, error)
}

// osDirFuncs is an implementation of dirFuncs that operates on the real
// filesystem.
type osDirFuncs struct {
	run runCommandFunc
}

func (*osDirFuncs) mkDirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (*osDirFuncs) lstat(path string) (fi os.FileInfo, err error) {
	return os.Lstat(path)
}

func (*osDirFuncs) fileCount(path string) (int, error) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return 0, errors.Annotate(err, "could not read directory")
	}
	return len(files), nil
}

func (*osDirFuncs) symlink(oldpath, newpath string) error {
	return os.Symlink(oldpath, newpath)
}

func (o *osDirFuncs) calculateSize(path string) (sizeInMib uint64, _ error) {
	output, err := df(o.run, path, "size")
	if err != nil {
		return 0, errors.Annotate(err, "getting size")
	}
	numBlocks, err := strconv.ParseUint(output, 10, 64)
	if err != nil {
		return 0, errors.Annotate(err, "parsing size")
	}
	return numBlocks / 1024, nil
}

func (o *osDirFuncs) bindMount(source, target string) error {
	_, err := o.run("mount", "--bind", source, target)
	return err
}

func (o *osDirFuncs) mountPoint(path string) (string, error) {
	target, err := df(o.run, path, "target")
	return target, err
}

func (o *osDirFuncs) mountPointSource(path string) (string, error) {
	source, err := df(o.run, path, "source")
	return source, err
}

func df(run runCommandFunc, path, field string) (string, error) {
	output, err := run("df", "--output="+field, path)
	if err != nil {
		return "", errors.Trace(err)
	}
	// the first line contains the headers
	lines := strings.SplitN(output, "\n", 2)
	return strings.TrimSpace(lines[1]), nil
}
