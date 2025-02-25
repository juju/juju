// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

import (
	stdtesting "testing"

	"go.uber.org/goleak"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package remote -destination remote_mock_test.go github.com/juju/juju/internal/objectstore/remote BlobsClient
//go:generate go run go.uber.org/mock/mockgen -typed -package remote -destination apiremotecaller_mock_test.go github.com/juju/juju/internal/worker/apiremotecaller APIRemoteCallers,RemoteConnection
//go:generate go run go.uber.org/mock/mockgen -typed -package remote -destination connection_mock_test.go github.com/juju/juju/api Connection
//go:generate go run go.uber.org/mock/mockgen -typed -package remote -destination clock_mock_test.go github.com/juju/clock Clock

func TestAll(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}
