// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package sshsession -destination ./service_mock_test.go github.com/juju/juju/internal/worker/sshsession FacadeClient,ConnectionGetter
//go:generate go run go.uber.org/mock/mockgen -package sshsession -destination ./stringswatcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher
//go:generate go run go.uber.org/mock/mockgen -package sshsession -destination ./agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -package sshsession -destination ./ephemeral_keys_updater_mock_test.go github.com/juju/juju/internal/worker/authenticationworker EphemeralKeysUpdater

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
