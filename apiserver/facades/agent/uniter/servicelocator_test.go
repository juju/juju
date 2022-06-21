// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/uniter/mocks"
	"github.com/juju/testing"
)

type serviceLocatorSuite struct {
	testing.IsolationSuite

	backend *mocks.MockServiceLocatorBackend
}

var _ = gc.Suite(&serviceLocatorSuite{})
