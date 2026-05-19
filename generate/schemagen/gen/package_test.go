// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gen_test

//go:generate go run github.com/canonical/gomock/mockgen -package gen -destination describeapi_mock.go -write_package_comment=false github.com/juju/juju/generate/schemagen/gen APIServer,Registry,PackageRegistry
