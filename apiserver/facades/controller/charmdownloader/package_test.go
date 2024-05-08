// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mocks.go github.com/juju/juju/apiserver/facades/controller/charmdownloader StateBackend,ModelBackend,Application,Charm,Downloader,AuthChecker,ResourcesBackend
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mock_watcher.go github.com/juju/juju/state StringsWatcher

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
