// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/broker_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner CAASBroker
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/facade_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner CAASProvisionerFacade
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/domain_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner ApplicationService,StatusService,AgentPasswordService,StorageProvisioningService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/worker_mocks.go github.com/juju/juju/internal/worker/caasapplicationprovisioner Runner,ResourceOpenerGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/ops_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner ApplicationOps
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/services_mocks.go github.com/juju/juju/internal/services ModelDomainServices
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/resource_mock.go github.com/juju/juju/core/resource Opener

var NewProvisionerWorkerForTest = newProvisionerWorker
var AppOps = &applicationOps{}
