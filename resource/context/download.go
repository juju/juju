// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

// TODO(ericsnow) Move this file elsewhere?
//  (e.g. top-level resource pkg, charm/resource)

import (
	"fmt"
	"io"
	"os"

	"github.com/juju/errors"
)

type downloader struct {
	downloaderDeps

	dirSpec    resourceDirectorySpec
	remote     *openedResource
	isUpToDate bool
}

func (dl downloader) Close() error {
	var errs multiErr

	if err := dl.dirSpec.CleanUp(); err != nil {
		err = errors.Annotate(err, "could not clean up temp dir")
		errs.add(err)
	}

	if dl.remote != nil {
		if err := dl.remote.Close(); err != nil {
			errs.add(errors.Trace(err))
		}
	}

	return errs.resolve()
}

func (dl downloader) path() string {
	if dl.remote == nil {
		return ""
	}
	return dl.remote.Path
}

func (dl downloader) download() (*resourceDirectory, error) {
	if dl.isUpToDate {
		return nil, nil // This indicates that nothing was downloaded.
	}

	target, err := dl.dirSpec.open(dl.mkdirAll)
	if err != nil {
		return nil, errors.Trace(err)
	}

	content := dl.remote.content()
	relPath := []string{dl.path()}
	if err := target.writeResource(relPath, content, dl.createFile); err != nil {
		return nil, errors.Trace(err)
	}

	return target, nil
}

type downloaderDeps struct {
	mkdirAll   func(string) error
	createFile func(string) (io.WriteCloser, error)
}

func newDownloaderDeps() downloaderDeps {
	return downloaderDeps{
		mkdirAll:   func(path string) error { return os.MkdirAll(path, 0755) },
		createFile: func(path string) (io.WriteCloser, error) { return os.Create(path) },
	}
}

// TODO(ericsnow) Try landing this is the errors repo (again).

type multiErr struct {
	errs []error
}

func (me *multiErr) add(err error) {
	me.errs = append(me.errs, err)
}

func (me multiErr) resolve() error {
	switch len(me.errs) {
	case 0:
		return nil
	case 1:
		return me.errs[0]
	default:
		msg := fmt.Sprintf("got %d errors:", len(me.errs))
		for i, err := range me.errs {
			msg = fmt.Sprintf("%s\n(#%d) %v", msg, i, err)
		}
		return errors.Errorf(msg)
	}
}
