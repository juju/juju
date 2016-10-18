// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/environs/gui"
	"github.com/juju/juju/testing"
)

type guiSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&guiSuite{})

func (s *guiSuite) TestGUIDataSourceBaseURL(c *gc.C) {
	c.Assert(common.GUIDataSourceBaseURL(), gc.Equals, gui.DefaultBaseURL)
	url := "https://1.2.3.4/streams/gui"
	s.PatchEnvironment("JUJU_GUI_SIMPLESTREAMS_URL", url)
	c.Assert(common.GUIDataSourceBaseURL(), gc.Equals, url)
}
