// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package raftlease implements the API for sending raft lease messages between
// api servers.
package raftlease_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package raftlease -destination remote_mock.go github.com/juju/juju/api/raftlease Remote,RaftLeaseApplier

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
