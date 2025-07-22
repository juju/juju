// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools_test

//go:generate go run go.uber.org/mock/mockgen -typed -package agenttools -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/agenttools MachineService,ModelConfigService,ModelAgentService
//go:generate go run go.uber.org/mock/mockgen -typed -package agenttools -destination environs_mock_test.go github.com/juju/juju/environs BootstrapEnviron
