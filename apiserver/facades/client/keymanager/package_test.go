// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

//go:generate go run github.com/canonical/gomock/mockgen -package keymanager -destination keymanager_mocks_test.go github.com/juju/juju/apiserver/facades/client/keymanager BlockChecker
//go:generate go run github.com/canonical/gomock/mockgen -package keymanager -destination service_mocks_test.go github.com/juju/juju/apiserver/facades/client/keymanager KeyManagerService,UserService
