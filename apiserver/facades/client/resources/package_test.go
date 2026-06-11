// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

//go:generate go run github.com/canonical/gomock/mockgen -package resources -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/resources ApplicationService,ResourceService,CrossModelRelationService,NewCharmRepository
