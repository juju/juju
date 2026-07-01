// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

//go:generate go run go.uber.org/mock/mockgen -typed -package unitless -self_package github.com/juju/juju/internal/worker/scriptlet -destination service_mock_test.go github.com/juju/juju/internal/worker/scriptlet ScriptletService
