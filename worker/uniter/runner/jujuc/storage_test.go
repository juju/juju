// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	gc "gopkg.in/check.v1"

	hookstesting "github.com/juju/juju/worker/common/hooks/testing"
)

var (
	storageAttributes = map[string]interface{}{
		"location": "/dev/sda",
		"kind":     "block",
	}

	storageName = "data/0"
)

type storageSuite struct {
	hookstesting.ContextSuite

	storageName string
	location    string
}

func (s *storageSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)

	s.storageName = "data/0"
	s.location = "/dev/sda"
}

func (s *storageSuite) newHookContext() (*hookstesting.Context, *hookstesting.ContextInfo) {
	hctx, info := s.ContextSuite.NewHookContextAndInfo()
	info.SetBlockStorage(s.storageName, s.location, s.Stub)
	info.SetStorageTag(s.storageName)
	return hctx, info
}
