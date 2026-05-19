// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/facade_mocks.go github.com/juju/juju/internal/worker/firewaller FirewallerAPI,CrossModelFirewallerFacadeCloser,EnvironFirewaller,EnvironModelFirewaller,EnvironInstances,EnvironInstance
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/entity_mocks.go github.com/juju/juju/internal/worker/firewaller Machine,Unit,Application
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/domain_mocks.go github.com/juju/juju/internal/worker/firewaller PortService,ApplicationService,CrossModelRelationService,RelationService
