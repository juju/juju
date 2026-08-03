// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/internal/testing"
)

type snapBuildSuite struct {
	coretesting.BaseSuite
}

func TestSnapBuildSuite(t *testing.T) {
	tc.Run(t, &snapBuildSuite{})
}

func (s *snapBuildSuite) TestSnapArchIdentity(c *tc.C) {
	c.Check(bootstrap.SnapArch("amd64"), tc.Equals, "amd64")
	c.Check(bootstrap.SnapArch("arm64"), tc.Equals, "arm64")
	c.Check(bootstrap.SnapArch("s390x"), tc.Equals, "s390x")
}

func (s *snapBuildSuite) TestSnapArchMapping(c *tc.C) {
	c.Check(bootstrap.SnapArch("arm"), tc.Equals, "armhf")
	c.Check(bootstrap.SnapArch("ppc64le"), tc.Equals, "ppc64el")
}

func (s *snapBuildSuite) TestSnapArchUnknown(c *tc.C) {
	c.Check(bootstrap.SnapArch("riscv64"), tc.Equals, "riscv64")
}

func (s *snapBuildSuite) TestBuildControllerSnapSnapcraftNotFound(c *tc.C) {
	origBuild := bootstrap.BuildCommandFunc
	restoreBuild := func() { *bootstrap.BuildCommandFunc = *origBuild }
	defer restoreBuild()

	*bootstrap.BuildCommandFunc = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("should not be called")
	}

	origLookPath := bootstrap.LookPathFunc
	restoreLookPath := func() { *bootstrap.LookPathFunc = *origLookPath }
	defer restoreLookPath()
	*bootstrap.LookPathFunc = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}

	_, err := bootstrap.BuildControllerSnap(context.Background())
	c.Assert(err, tc.ErrorMatches, "snapcraft is required to build the controller snap.*")
}

func (s *snapBuildSuite) TestBuildControllerSnapMakeFails(c *tc.C) {
	origBuild := bootstrap.BuildCommandFunc
	restoreBuild := func() { *bootstrap.BuildCommandFunc = *origBuild }
	defer restoreBuild()

	*bootstrap.BuildCommandFunc = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		return []byte("build output\nbuild failed\n"), fmt.Errorf("exit status 2")
	}

	origLookPath := bootstrap.LookPathFunc
	restoreLookPath := func() { *bootstrap.LookPathFunc = *origLookPath }
	defer restoreLookPath()
	*bootstrap.LookPathFunc = func(file string) (string, error) {
		return "/usr/bin/snapcraft", nil
	}

	_, err := bootstrap.BuildControllerSnap(context.Background())
	c.Assert(err, tc.ErrorMatches, "building controller snap failed: exit status 2(?s).*")
}

func (s *snapBuildSuite) TestBuildControllerSnapFileNotFound(c *tc.C) {
	tmpDir := c.MkDir()

	origBuild := bootstrap.BuildCommandFunc
	restoreBuild := func() { *bootstrap.BuildCommandFunc = *origBuild }
	defer restoreBuild()

	*bootstrap.BuildCommandFunc = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		return []byte("build complete\n"), nil
	}

	origLookPath := bootstrap.LookPathFunc
	restoreLookPath := func() { *bootstrap.LookPathFunc = *origLookPath }
	defer restoreLookPath()
	*bootstrap.LookPathFunc = func(file string) (string, error) {
		return "/usr/bin/snapcraft", nil
	}

	origFindSourceRoot := tools.FindJujuSourceRoot
	restoreFindSourceRoot := func() { tools.FindJujuSourceRoot = origFindSourceRoot }
	defer restoreFindSourceRoot()
	tools.FindJujuSourceRoot = func() (string, error) {
		return tmpDir, nil
	}

	_, err := bootstrap.BuildControllerSnap(context.Background())
	c.Assert(err, tc.ErrorMatches, "controller snap build completed but expected file.*was not found.*")
}

func (s *snapBuildSuite) TestBuildControllerSnapSuccess(c *tc.C) {
	tmpDir := c.MkDir()

	snapName := fmt.Sprintf("jujud_%s_%s.snap", "4.1-beta2", runtime.GOARCH)
	snapPath := filepath.Join(tmpDir, snapName)
	err := os.WriteFile(snapPath, []byte("fake snap"), 0644)
	c.Assert(err, tc.ErrorIsNil)

	origBuild := bootstrap.BuildCommandFunc
	restoreBuild := func() { *bootstrap.BuildCommandFunc = *origBuild }
	defer restoreBuild()

	buildCalled := false
	*bootstrap.BuildCommandFunc = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		c.Check(dir, tc.Equals, tmpDir)
		c.Check(name, tc.Equals, "make")
		c.Check(args, tc.DeepEquals, []string{"build-snap"})
		buildCalled = true
		return []byte("build complete\n"), nil
	}

	origLookPath := bootstrap.LookPathFunc
	restoreLookPath := func() { *bootstrap.LookPathFunc = *origLookPath }
	defer restoreLookPath()
	*bootstrap.LookPathFunc = func(file string) (string, error) {
		return "/usr/bin/snapcraft", nil
	}

	origFindSourceRoot := tools.FindJujuSourceRoot
	restoreFindSourceRoot := func() { tools.FindJujuSourceRoot = origFindSourceRoot }
	defer restoreFindSourceRoot()
	tools.FindJujuSourceRoot = func() (string, error) {
		return tmpDir, nil
	}

	result, err := bootstrap.BuildControllerSnap(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(buildCalled, tc.IsTrue)
	c.Check(result, tc.Equals, snapPath)
}
