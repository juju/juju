// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package externalcontrollerupdater_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/controller/externalcontrollerupdater ExternalControllerService
//go:generate go run go.uber.org/mock/mockgen -typed -package externalcontrollerupdater_test -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher

