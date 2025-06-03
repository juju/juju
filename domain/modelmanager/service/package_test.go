// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/modelmanager/service ModelRemover,ModelTypeState,State,WatchableState,WatcherFactory

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
