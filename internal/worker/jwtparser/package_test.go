// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

//go:generate go run github.com/canonical/gomock/mockgen -package jwtparser -destination service_mocks_test.go github.com/juju/juju/internal/worker/jwtparser ControllerConfigService,HTTPClient
