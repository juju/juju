// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package charmdownloader -destination mocks.go github.com/juju/juju/apiserver/facades/controller/charmdownloader StateBackend,ModelBackend,Application,Charm,Downloader,AuthChecker,ResourcesBackend
//go:generate go run github.com/golang/mock/mockgen -package charmdownloader -destination mock_watcher.go github.com/juju/juju/state StringsWatcher

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
