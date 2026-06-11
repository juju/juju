// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package offererrelations

//go:generate go run github.com/canonical/gomock/mockgen -package offererrelations -destination client_mock_test.go github.com/juju/juju/internal/worker/remoterelationconsumer/offererrelations RemoteModelRelationsClient,ReportableWorker
