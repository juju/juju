// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v200 = version.MustParse("2.0.0")

type steps20Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps20Suite{})

func (s *steps20Suite) TestStripLocalUserDomain(c *gc.C) {
	step := findStateStep(c, v200, "strip @local from local user names")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps20Suite) TestRenameAddModelPermission(c *gc.C) {
	step := findStateStep(c, v200, "rename addmodel permission to add-model")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps20Suite) TestCharmGetCacheDir(c *gc.C) {
	// Create a cache directory with some stuff in it.
	dataDir := c.MkDir()
	cacheDir := filepath.Join(dataDir, "charm-get-cache")
	c.Assert(os.MkdirAll(cacheDir, 0777), jc.ErrorIsNil)
	err := ioutil.WriteFile(filepath.Join(cacheDir, "stuff"), []byte("things"), 0777)
	c.Assert(err, jc.ErrorIsNil)

	step := findStep(c, v200, "remove apiserver charm get cache")

	check := func() {
		context := &mockContext{
			agentConfig: &mockAgentConfig{dataDir: dataDir},
		}
		err = step.Run(context)
		c.Assert(err, jc.ErrorIsNil)

		// Cache directory should be gone, but data dir should still be there.
		c.Check(pathExists(cacheDir), jc.IsFalse)
		c.Check(pathExists(dataDir), jc.IsTrue)
	}

	check()
	check() // Check OK when directory not present
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	if err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false
	}
	panic(fmt.Sprintf("stat for %q failed: %v", p, err))
}
