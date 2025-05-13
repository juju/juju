// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner_test

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package cleaner_test -destination domainservices_mock_test.go github.com/juju/juju/internal/services DomainServices

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
