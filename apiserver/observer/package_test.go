// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer_test

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package observer -destination services_mock_test.go github.com/juju/juju/apiserver/observer DomainServicesGetter,ModelService,StatusService

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
