// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
)

// The base suite for backups testing.
type BaseSuite struct {
	testing.IsolationSuite
	// Meta is a Metadata with standard test values.
	Meta *backups.Metadata
	// Storage is a FileStorage to use when testing backups.
	Storage *FakeStorage
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Meta = NewMetadata()
	s.Storage = &FakeStorage{}
}
