// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination mock_test.go github.com/juju/juju/domain/agentprovisioner/service State,Provider

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
