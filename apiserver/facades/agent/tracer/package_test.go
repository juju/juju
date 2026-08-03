// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer_test

//go:generate go run github.com/canonical/gomock/mockgen -package tracer_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/tracer ControllerTracingConfigService
