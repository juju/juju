// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -mock_names=Broker=MockExtCAASBroker -destination mocks/caasbroker_mock.go github.com/juju/juju/caas Broker
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/broker_mock.go github.com/juju/juju/internal/worker/caasfirewaller CAASBroker,PortMutator
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/worker_mock.go github.com/juju/worker/v4 Worker
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/domain_mocks.go github.com/juju/juju/internal/worker/caasfirewaller ApplicationService,PortService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/services_mocks.go github.com/juju/juju/internal/services ModelDomainServices
