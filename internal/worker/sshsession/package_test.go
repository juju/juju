// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package sshsession_test -destination ./facade_client_mock_test.go github.com/juju/juju/internal/worker/sshsession FacadeClient,Logger
//go:generate go run go.uber.org/mock/mockgen -package sshsession_test -destination ./stringswatcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
