// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"io/ioutil"
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
