// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination store_mock_test.go github.com/juju/juju/domain/agentbinary/service State,AgentBinaryState,ProviderForAgentBinaryFinder
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ModelObjectStoreGetter,ObjectStore
func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
