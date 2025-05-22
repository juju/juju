// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

//go:generate go run go.uber.org/mock/mockgen -package jwtparser -destination service_mock.go github.com/juju/juju/internal/worker/jwtparser ControllerConfigService,HTTPClient
