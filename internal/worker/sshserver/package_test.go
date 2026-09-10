// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

//go:generate go run github.com/canonical/gomock/mockgen -package sshserver -destination service_mock_test.go github.com/juju/juju/internal/worker/sshserver ControllerConfigService,Authenticator,Authorizer
//go:generate go run github.com/canonical/gomock/mockgen -package sshserver -destination proxy_mock_test.go github.com/juju/juju/internal/sshproxy ProxyFactory,ProxyHandlers,Resolver
//go:generate go run github.com/canonical/gomock/mockgen -package sshserver -destination listener_mock_test.go net Listener
