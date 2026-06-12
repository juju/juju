// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs_test

//go:generate go run github.com/canonical/gomock/mockgen -package httpserverargs -destination services_mock_test.go github.com/juju/juju/internal/worker/httpserverargs ControllerConfigService,AccessService,DomainServicesGetter,ModelService
