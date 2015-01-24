// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/juju/errors"
)

func listSubdirectories(dirname string) ([]string, error) {
	entries, err := ioutil.ReadDir(dirname)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var dirnames []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirnames = append(dirnames, entry.Name())
	}
	return dirnames, nil
}

func hasPrefix(name string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func contains(strList []string, str string) bool {
	for _, contained := range strList {
		if str == contained {
			return true
		}
	}
	return false
}

// TODO(ericsnow) Move this to the utils repo.

type fileOperations interface {
	exists(name string) (bool, error)
	mkdirAll(dirname string, mode os.FileMode) error
	readFile(filename string) ([]byte, error)
	createFile(filename string) (io.WriteCloser, error)
	removeAll(name string) error
	chmod(name string, mode os.FileMode) error
}
type fileOps struct{}

func (fileOps) exists(name string) (bool, error) {
	_, err := os.Stat(name)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (fileOps) mkdirAll(dirname string, mode os.FileMode) error {
	return os.MkdirAll(dirname, mode)
}

func (fileOps) readFile(filename string) ([]byte, error) {
	return ioutil.ReadFile(filename)
}

func (fileOps) createFile(filename string) (io.WriteCloser, error) {
	return os.Create(filename)
}

func (fileOps) removeAll(name string) error {
	return os.RemoveAll(name)
}

func (fileOps) chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}
