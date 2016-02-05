// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package spool_test

import (
	"io"

	"github.com/juju/errors"
	"gopkg.in/natefinch/npipe.v2"
)

func dial(socketPath string) (io.ReadCloser, error) {
	conn, err := npipe.Dial(socketPath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conn, nil
}
