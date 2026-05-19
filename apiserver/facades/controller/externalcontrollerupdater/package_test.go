// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

//go:generate go run github.com/canonical/gomock/mockgen -package externalcontrollerupdater_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/controller/externalcontrollerupdater ExternalControllerService
//go:generate go run github.com/canonical/gomock/mockgen -package externalcontrollerupdater_test -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher
