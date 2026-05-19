// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

//go:generate go run github.com/canonical/gomock/mockgen -package authentication_test -destination package_mock_test.go github.com/juju/juju/apiserver/authentication AgentPasswordService
