// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

//go:generate go run go.uber.org/mock/mockgen -package proxyupdater_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/proxyupdater ControllerNodeService,ModelConfigService
