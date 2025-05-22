// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

//go:generate go run go.uber.org/mock/mockgen -typed -package remote -destination remote_mock_test.go github.com/juju/juju/internal/objectstore/remote BlobsClient
//go:generate go run go.uber.org/mock/mockgen -typed -package remote -destination apiremotecaller_mock_test.go github.com/juju/juju/internal/worker/apiremotecaller APIRemoteCallers,RemoteConnection
//go:generate go run go.uber.org/mock/mockgen -typed -package remote -destination connection_mock_test.go github.com/juju/juju/api Connection
//go:generate go run go.uber.org/mock/mockgen -typed -package remote -destination clock_mock_test.go github.com/juju/clock Clock
