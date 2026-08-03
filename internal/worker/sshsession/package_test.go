// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

//go:generate go run github.com/canonical/gomock/mockgen -package sshsession -destination service_mock_test.go github.com/juju/juju/internal/worker/sshsession FacadeClient,ConnectionDialer
//go:generate go run github.com/canonical/gomock/mockgen -package sshsession -destination coressh_mock_test.go github.com/juju/juju/core/ssh EphemeralKeysUpdater
