// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/moby/sys/mountinfo"
)

// dirFuncs is used to allow the real directory operations to
// be stubbed out for testing.
type dirFuncs interface {
	etcDir() string
	mkDirAll(path string, perm os.FileMode) error
	lstat(path string) (fi os.FileInfo, err error)
	fileCount(path string) (int, error)
	calculateSize(path string) (sizeInMib uint64, _ error)

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
	run          RunCommandFunc
	mountInfoRdr io.ReadSeeker
}

func (*osDirFuncs) etcDir() string {
	return "/etc"
}

func (*osDirFuncs) mkDirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (*osDirFuncs) lstat(path string) (fi os.FileInfo, err error) {
	return os.Lstat(path)
}

func (*osDirFuncs) fileCount(path string) (int, error) {
	files, err := os.ReadDir(path)
	if err != nil {
		return 0, errors.Annotate(err, "could not read directory")
	}
	return len(files), nil
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

const mountInfoPath = "/proc/self/mountinfo"

func (o *osDirFuncs) infoReader() (io.ReadSeeker, func(), error) {
	if o.mountInfoRdr == nil {
		f, err := os.Open(mountInfoPath)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		return f, func() {
			_ = f.Close()
		}, nil
	}
	return o.mountInfoRdr, func() {
		_, _ = o.mountInfoRdr.Seek(0, 0)
	}, nil
}

func (o *osDirFuncs) mountPoint(path string) (string, error) {
	infoReader, closer, err := o.infoReader()
	if err != nil {
		return "", errors.Annotatef(err, "opening %s", mountInfoPath)
	}
	defer closer()
	mi, err := getMountsFromReader(infoReader, mountinfo.SingleEntryFilter(path))
	if err != nil {
		return "", errors.Annotatef(err, "getting info for mount point %q", path)
	}
	if len(mi) == 0 {
		return "", nil
	}
	return mi[0].Source, nil
}

func (o *osDirFuncs) mountPointSource(target string) (result string, _ error) {
	defer func() {
		logger.Debugf(context.TODO(), "mount point source for %q is %q", target, result)
	}()

	infoReader, closer, err := o.infoReader()
	if err != nil {
		return "", errors.Annotate(err, "opening /proc/self/mountinfo")
	}
	defer closer()

	mi, err := getMountsFromReader(infoReader, nil)
	if err != nil {
		return "", errors.Annotatef(err, "getting info for mount point %q", target)
	}
	if len(mi) == 0 {
		return "", nil
	}

	mountsById := make(map[int]*mountinfo.Info)
	var targetInfo *mountinfo.Info
	for _, m := range mi {
		mountsById[m.ID] = m
		if m.Mountpoint == target {
			mp := *m
			targetInfo = &mp
		}
	}
	if targetInfo == nil {
		return "", nil
	}

	if targetInfo.FSType == "tmpfs" {
		return targetInfo.Source, nil
	}

	// Step through any parent records to progressively
	// resolve the absolute source path.
	source := targetInfo.Root
	for {
		next, ok := mountsById[targetInfo.Parent]
		if !ok {
			break
		}
		next.Root = strings.TrimSuffix(next.Root, "/")
		next.Mountpoint = strings.TrimSuffix(next.Mountpoint, "/")
		if strings.HasPrefix(source, next.Root+"/") {
			source = strings.Replace(source, next.Root, next.Mountpoint, 1)
		}
		targetInfo = next
	}
	return source, nil
}

func df(run RunCommandFunc, path, field string) (string, error) {
	output, err := run("df", "--output="+field, path)
	if err != nil {
		return "", errors.Trace(err)
	}
	// the first line contains the headers
	lines := strings.SplitN(output, "\n", 2)
	return strings.TrimSpace(lines[1]), nil
}
