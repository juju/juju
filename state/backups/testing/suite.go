// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups/metadata"
)

// The base suite for backups testing.
type BaseSuite struct {
	testing.IsolationSuite
	Meta *metadata.Metadata
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Meta = s.metadata()
}

func (s *BaseSuite) metadata() *metadata.Metadata {
	return NewMetadata()
}
