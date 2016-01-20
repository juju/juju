// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect_test

// +build !windows

import (
	"io"
	"net"

	"github.com/juju/errors"
)

func dial(socketPath string) (io.ReadCloser, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conn, nil
}
