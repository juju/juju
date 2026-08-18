// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

//go:generate go run github.com/canonical/gomock/mockgen -package model_test -destination state_mock_test.go github.com/juju/juju/domain/ssh/service/model State
