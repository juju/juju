// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package externalcontrollerupdater_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/controller/externalcontrollerupdater ECService
//go:generate go run go.uber.org/mock/mockgen -typed -package externalcontrollerupdater_test -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
