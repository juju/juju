// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerpresence

//go:generate go run github.com/canonical/gomock/mockgen -package controllerpresence -destination package_mocks_test.go github.com/juju/juju/internal/worker/controllerpresence ControllerDomainServices,DomainServices,ModelService,StatusService
//go:generate go run github.com/canonical/gomock/mockgen -package controllerpresence -destination caller_mocks_test.go github.com/juju/juju/internal/worker/apiremotecaller APIRemoteSubscriber,Subscription,RemoteConnection
//go:generate go run github.com/canonical/gomock/mockgen -package controllerpresence -destination api_mocks_test.go github.com/juju/juju/api Connection
