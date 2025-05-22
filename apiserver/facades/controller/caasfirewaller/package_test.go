// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

//go:generate go run go.uber.org/mock/mockgen -typed -package caasfirewaller_test -destination common_service_mocks_test.go github.com/juju/juju/apiserver/internal/charms CharmService
//go:generate go run go.uber.org/mock/mockgen -typed -package caasfirewaller_test -destination service_mocks_test.go github.com/juju/juju/apiserver/facades/controller/caasfirewaller ApplicationService
