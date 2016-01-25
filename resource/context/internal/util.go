// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"io"

	"github.com/juju/errors"
)

// Logger exposes the logger functionality needed by CloseAndLog.
type Logger interface {
	// Errorf formats the provided log message and writes it to the log.
	Errorf(string, ...interface{})
}

// CloseAndLog calls the closer's Close() and logs any error returned therefrom.
func CloseAndLog(closer io.Closer, label string, logger Logger) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		logger.Errorf("while closing %s: %v", label, err)
	}
}

// ReplaceDirectory replaces the target directory with the source. This
// involves removing the target if it exists and then moving the source
// into place.
func ReplaceDirectory(targetDir, sourceDir string, deps ReplaceDirectoryDeps) error {
	// TODO(ericsnow) Move it out of the way and remove it after the rename.
	if err := deps.RemoveDir(targetDir); err != nil {
		return errors.Trace(err)
	}
	if err := deps.Move(targetDir, sourceDir); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ReplaceDirectoryDeps exposes the functionality needed by ReplaceDirectory.
type ReplaceDirectoryDeps interface {
	// RemoveDir deletes the directory at the given path.
	RemoveDir(dirname string) error

	// Move moves the directory at the source path to the target path.
	Move(target, source string) error
}

// TODO(ericsnow) Try landing this is the errors repo (again).

//type multiErr struct {
//	errs []error
//}
//
//func (me *multiErr) add(err error) {
//	me.errs = append(me.errs, err)
//}
//
//func (me multiErr) resolve() error {
//	switch len(me.errs) {
//	case 0:
//		return nil
//	case 1:
//		return me.errs[0]
//	default:
//		msg := fmt.Sprintf("got %d errors:", len(me.errs))
//		for i, err := range me.errs {
//			msg = fmt.Sprintf("%s\n(#%d) %v", msg, i, err)
//		}
//		return errors.Errorf(msg)
//	}
//}
