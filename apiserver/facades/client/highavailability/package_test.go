// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

//go:generate go run github.com/canonical/gomock/mockgen -package highavailability -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/highavailability ControllerNodeService
//go:generate go run github.com/canonical/gomock/mockgen -package highavailability -destination auth_mock_test.go github.com/juju/juju/apiserver/facade Authorizer
