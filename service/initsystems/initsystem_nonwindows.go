// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package initsystems

import (
	"io/ioutil"
	"strings"

	"github.com/juju/errors"
)

func findInitExecutable() (string, error) {
	// This should work on all linux-like OSes.
	data, err := ioutil.ReadFile("/proc/1/cmdline")
	if err != nil {
		return "", errors.Trace(err)
	}
	return strings.Fields(string(data))[0], nil
}
