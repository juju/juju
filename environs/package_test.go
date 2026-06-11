// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/environs Environ,NetworkingEnviron,CloudEnvironProvider,InstanceTypesFetcher
