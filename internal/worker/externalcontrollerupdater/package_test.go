// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

//go:generate go run github.com/canonical/gomock/mockgen -package externalcontrollerupdater -destination package_mock_test.go github.com/juju/juju/internal/worker/externalcontrollerupdater ExternalControllerWatcherClientCloser,ExternalControllerService
