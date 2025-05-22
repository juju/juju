// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

//go:generate go run go.uber.org/mock/mockgen -typed -package applicationoffers_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/applicationoffers AccessService,ApplicationService,ModelDomainServicesGetter,ModelDomainServices,ModelService
