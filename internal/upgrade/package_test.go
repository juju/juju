// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

//go:generate go run github.com/canonical/gomock/mockgen -package upgrade -destination lock_mock_test.go github.com/juju/juju/internal/worker/gate Lock
//go:generate go run github.com/canonical/gomock/mockgen -package upgrade -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
