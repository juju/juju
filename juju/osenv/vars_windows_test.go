// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"path/filepath"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
)

func (s *varsSuite) TestJujuHome(c *gc.C) {
	path := `P:\FooBar\AppData`
	s.PatchEnvironment("APPDATA", path)
	c.Assert(osenv.JujuHomeWin(), gc.Equals, filepath.Join(path, "Juju"))
}
