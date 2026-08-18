// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

//go:generate go run github.com/canonical/gomock/mockgen -package controller_test -destination state_mock_test.go github.com/juju/juju/domain/ssh/service/controller State
