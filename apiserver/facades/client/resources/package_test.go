// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

//go:generate go run go.uber.org/mock/mockgen -typed -package resources -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/resources ApplicationService,ResourceService,NewCharmRepository
