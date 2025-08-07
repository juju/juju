// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

//go:generate go run go.uber.org/mock/mockgen -typed -package externalcontrollerupdater -destination package_mock_test.go github.com/juju/juju/internal/worker/externalcontrollerupdater ExternalControllerWatcherClientCloser,ExternalControllerUpdaterClient
