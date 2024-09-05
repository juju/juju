// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination proxy_mock_test.go github.com/juju/juju/internal/proxy Proxier
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination provider_mock_test.go github.com/juju/juju/domain/proxy/service Provider

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
