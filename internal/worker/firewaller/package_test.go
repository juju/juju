// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/facade_mocks.go github.com/juju/juju/internal/worker/firewaller FirewallerAPI,RemoteRelationsAPI,CrossModelFirewallerFacadeCloser,EnvironFirewaller,EnvironModelFirewaller,EnvironInstances,EnvironInstance
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/entity_mocks.go github.com/juju/juju/internal/worker/firewaller Machine,Unit,Application
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/domain_mocks.go github.com/juju/juju/internal/worker/firewaller MachineService,PortService,ApplicationService
