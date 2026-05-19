// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

//go:generate go run github.com/canonical/gomock/mockgen -package keyupdater -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/keyupdater KeyUpdaterService
//go:generate go run github.com/canonical/gomock/mockgen -package keyupdater -destination facade_mock_test.go github.com/juju/juju/apiserver/facade ModelContext
