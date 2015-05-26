// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd

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
	CurrentEnvironmentFilename = "current-environment"
	CurrentSystemFilename      = "current-system"

	lockName = "current.lock"
)

var (
	// 5 seconds should be way more than enough to write or read any files
	// even on heavily loaded systems.
	lockTimeout = 5 * time.Second
)

// ServerFile describes the information that is needed for a user
// to connect to an api server.
type ServerFile struct {
	Addresses []string `yaml:"addresses"`
	CACert    string   `yaml:"ca-cert,omitempty"`
	Username  string   `yaml:"username"`
	Password  string   `yaml:"password"`
}

// NOTE: synchronisation across functions in this file.
//
// Each of the read and write functions use a fslock to synchronise calls
// across both the current executable and across different executables.

func getCurrentEnvironmentFilePath() string {
	return filepath.Join(osenv.JujuHome(), CurrentEnvironmentFilename)
}

func getCurrentSystemFilePath() string {
	return filepath.Join(osenv.JujuHome(), CurrentSystemFilename)
}

// Read the file $JUJU_HOME/current-environment and return the value stored
// there.  If the file doesn't exist an empty string is returned and no error.
func ReadCurrentEnvironment() (string, error) {
	lock, err := acquireEnvironmentLock("read current-environment")
	if err != nil {
		return "", errors.Trace(err)
	}
	defer lock.Unlock()

	current, err := ioutil.ReadFile(getCurrentEnvironmentFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Trace(err)
	}
	return strings.TrimSpace(string(current)), nil
}

// Read the file $JUJU_HOME/current-system and return the value stored there.
// If the file doesn't exist an empty string is returned and no error.
func ReadCurrentSystem() (string, error) {
	lock, err := acquireEnvironmentLock("read current-system")
	if err != nil {
		return "", errors.Trace(err)
	}
	defer lock.Unlock()

	current, err := ioutil.ReadFile(getCurrentSystemFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Trace(err)
	}
	return strings.TrimSpace(string(current)), nil
}

// Write the envName to the file $JUJU_HOME/current-environment file.
func WriteCurrentEnvironment(envName string) error {
	lock, err := acquireEnvironmentLock("write current-environment")
	if err != nil {
		return errors.Trace(err)
	}
	defer lock.Unlock()

	path := getCurrentEnvironmentFilePath()
	err = ioutil.WriteFile(path, []byte(envName+"\n"), 0644)
	if err != nil {
		return errors.Errorf("unable to write to the environment file: %q, %s", path, err)
	}
	// If there is a current system file, remove it.
	if err := os.Remove(getCurrentSystemFilePath()); err != nil && !os.IsNotExist(err) {
		logger.Debugf("removing the current environment file due to %s", err)
		// Best attempt to remove the file we just wrote.
		os.Remove(path)
		return err
	}
	return nil
}

// Write the systemName to the file $JUJU_HOME/current-system file.
func WriteCurrentSystem(systemName string) error {
	lock, err := acquireEnvironmentLock("write current-system")
	if err != nil {
		return errors.Trace(err)
	}
	defer lock.Unlock()

	path := getCurrentSystemFilePath()
	err = ioutil.WriteFile(path, []byte(systemName+"\n"), 0644)
	if err != nil {
		return errors.Errorf("unable to write to the system file: %q, %s", path, err)
	}
	// If there is a current environment file, remove it.
	if err := os.Remove(getCurrentEnvironmentFilePath()); err != nil && !os.IsNotExist(err) {
		logger.Debugf("removing the current system file due to %s", err)
		// Best attempt to remove the file we just wrote.
		os.Remove(path)
		return err
	}
	return nil
}

func acquireEnvironmentLock(operation string) (*fslock.Lock, error) {
	// NOTE: any reading or writing from the directory should be done with a
	// fslock to make sure we have a consistent read or write.  Also worth
	// noting, we should use a very short timeout.
	lock, err := fslock.NewLock(osenv.JujuHome(), lockName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = lock.LockWithTimeout(lockTimeout, operation)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return lock, nil
}
