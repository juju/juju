// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

//go:generate go run go.uber.org/mock/mockgen -typed -package crossmodelrelations_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/crossmodelrelations ModelConfigService,StatusService
