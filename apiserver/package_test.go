// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package apiserver -destination apiserver_mock_test.go github.com/juju/juju/apiserver Raft
//go:generate go run github.com/golang/mock/mockgen -package apiserver -destination raftlease_mock_test.go github.com/juju/juju/core/raftlease NotifyTarget,FSMResponse
//go:generate go run github.com/golang/mock/mockgen -package apiserver -destination raft_mock_test.go github.com/hashicorp/raft ApplyFuture,ConfigurationFuture

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
