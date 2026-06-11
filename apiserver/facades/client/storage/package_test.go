// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

//go:generate go run github.com/canonical/gomock/mockgen -package storage -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/client/storage ApplicationService,BlockChecker,MachineService,RemovalService,StatusService,StorageService
