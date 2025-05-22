// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext

//go:generate go run go.uber.org/mock/mockgen -typed -package resourceshookcontext -destination package_mock_test.go github.com/juju/juju/apiserver/facades/agent/resourceshookcontext ApplicationService,ResourceService
