// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongotest

import (
	"time"

	"github.com/juju/juju/internal/mongo"
)

const (
	DialTimeout   = 5 * time.Minute
	SocketTimeout = DialTimeout
)

// DialOpts returns mongo.DialOpts suitable for use in tests that operate
// against a real MongoDB server. The timeouts are chosen to avoid failures
// caused by slow I/O; we do not expect the timeouts to be reached under
// normal circumstances.
func DialOpts() mongo.DialOpts {
	return mongo.DialOpts{
		Timeout:       DialTimeout,
		SocketTimeout: SocketTimeout,
	}
}
