// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/fslock"

	"github.com/juju/juju/juju/osenv"
)

const (
	CurrentControllerFilename = "current-controller"

	lockName = "current.lock"
)

var (
	// 5 seconds should be way more than enough to write or read any files
	// even on heavily loaded controllers.
	lockTimeout = 5 * time.Second
)

// NOTE: synchronisation across functions in this file.
//
// Each of the read and write functions use a fslock to synchronise calls
// across both the current executable and across different executables.

func getCurrentControllerFilePath() string {
	return filepath.Join(osenv.JujuXDGDataHome(), CurrentControllerFilename)
}

// ReadCurrentController reads the file $JUJU_DATA/current-controller and
// return the value stored there. If the file doesn't exist an empty string is
// returned and no error.
func ReadCurrentController() (string, error) {
	lock, err := acquireEnvironmentLock("read current-controller")
	if err != nil {
		return "", errors.Trace(err)
	}
	defer lock.Unlock()

	current, err := ioutil.ReadFile(getCurrentControllerFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Trace(err)
	}
	return strings.TrimSpace(string(current)), nil
}

// WriteCurrentController writes the controllerName to the file
// $JUJU_DATA/current-controller file.
func WriteCurrentController(controllerName string) error {
	lock, err := acquireEnvironmentLock("write current-controller")
	if err != nil {
		return errors.Trace(err)
	}
	defer lock.Unlock()

	path := getCurrentControllerFilePath()
	err = ioutil.WriteFile(path, []byte(controllerName+"\n"), 0644)
	if err != nil {
		return errors.Errorf("unable to write to the controller file: %q, %s", path, err)
	}
	return nil
}

func acquireEnvironmentLock(operation string) (*fslock.Lock, error) {
	// NOTE: any reading or writing from the directory should be done with a
	// fslock to make sure we have a consistent read or write.  Also worth
	// noting, we should use a very short timeout.
	lock, err := fslock.NewLock(osenv.JujuXDGDataHome(), lockName, fslock.Defaults())
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = lock.LockWithTimeout(lockTimeout, operation)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return lock, nil
}
