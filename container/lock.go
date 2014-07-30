// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const (
	// NameRegexp specifies the regular expression used to identify valid lock names.
	NameRegexp = "^[a-z]+[a-z0-9.-]*$"
)

var (
	validName = regexp.MustCompile(NameRegexp)
)

type Lock struct {
	name     string
	lockDir  string
	lockFile *os.File
}

// NewLock returns a new lock with the given name, using the given lock
// directory, without acquiring it. The lock name must match the regular
// expression defined by NameRegexp.
func NewLock(dir, name string) (*Lock, error) {
	if !validName.MatchString(name) {
		return nil, fmt.Errorf("Invalid lock name %q.  Names must match %q", name, NameRegexp)
	}
	lockDir := filepath.Join(dir, name)
	lock := &Lock{
		name:    name,
		lockDir: lockDir,
	}
	// Ensure the lockDir exists.
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, err
	}
	return lock, nil
}

// Lock blocks until it is able to acquire the lock. It is good behaviour to
// provide a message that is output in debugging information.
func (lock *Lock) Lock(message string) (err error) {
	logger.Infof("acquire lock %q, %s", lock.name, message)
	lock.lockFile, err = os.Open(lock.lockDir)
	if err != nil {
		return err
	}
	fd := int(lock.lockFile.Fd())
	err = flockLock(fd)
	if err != nil {
		lock.lockFile.Close()
		lock.lockFile = nil
	}
	return err
}

// Unlock releases a held lock.
func (lock *Lock) Unlock() error {
	logger.Infof("release lock %q", lock.name)
	if lock.lockFile == nil {
		return nil
	}
	fd := int(lock.lockFile.Fd())
	err := flockUnlock(fd)
	if err == nil {
		lock.lockFile.Close()
		lock.lockFile = nil
	}
	return err
}
