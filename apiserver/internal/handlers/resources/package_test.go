// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package resources -destination resource_opener_mock_test.go github.com/juju/juju/core/resource Opener
//go:generate go run go.uber.org/mock/mockgen -typed -package resources -destination service_mock_test.go github.com/juju/juju/apiserver/internal/handlers/resources ResourceServiceGetter,ResourceService,ResourceOpenerGetter,Downloader

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
