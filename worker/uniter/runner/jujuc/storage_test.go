// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc/jujuctesting"
)

var (
	storageAttributes = map[string]interface{}{
		"location": "/dev/sda",
		"kind":     "block",
	}

	storageName = "data/0"
)

type storageSuite struct {
	jujuctesting.ContextSuite

	storageName string
	location    string
}

func (s *storageSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)

	s.storageName = "data/0"
	s.location = "/dev/sda"
}

func (s *storageSuite) newHookContext() (*jujuctesting.Context, *jujuctesting.ContextInfo) {
	hctx, info := s.ContextSuite.NewHookContext()
	info.SetBlockStorage(s.storageName, s.location, s.Stub)
	info.SetStorageTag(s.storageName)
	return hctx, info
}
