// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

//go:generate go run github.com/canonical/gomock/mockgen -package controlsocket -destination services_mock_test.go github.com/juju/juju/internal/worker/controlsocket AccessService,TracingService
