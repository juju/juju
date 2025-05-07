// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"path/filepath"

	"github.com/juju/tc"

	"github.com/juju/juju/juju/osenv"
)

func (s *varsSuite) TestJujuXDGDataHome(c *tc.C) {
	path := `P:\FooBar\AppData`
	s.PatchEnvironment("APPDATA", path)
	c.Assert(osenv.JujuXDGDataHomeWin(), tc.Equals, filepath.Join(path, "Juju"))
}
