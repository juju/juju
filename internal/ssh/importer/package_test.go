// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package importer

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package importer -destination http_mock_test.go github.com/juju/juju/internal/ssh/importer Client
//go:generate go run go.uber.org/mock/mockgen -typed -package importer -destination resolver_mock_test.go github.com/juju/juju/internal/ssh/importer Resolver

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
