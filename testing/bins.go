// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"sync"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type buildDetails struct {
	Pkg     string
	Bin     string
	BuildID string
}

var binMap = map[string]buildDetails{
	"jujud": {
		Pkg:     "github.com/juju/juju/cmd/jujud",
		Bin:     "jujud",
		BuildID: os.Getenv("JUJU_TESTING_JUJUD_BUILDID"),
	},
	"jujuc": {
		Pkg:     "github.com/juju/juju/cmd/jujuc",
		Bin:     "jujuc",
		BuildID: os.Getenv("JUJU_TESTING_JUJUC_BUILDID"),
	},
	"juju": {
		Pkg:     "github.com/juju/juju/cmd/juju",
		Bin:     "juju",
		BuildID: os.Getenv("JUJU_TESTING_JUJU_BUILDID"),
	},
}

var (
	modParamCacheMutex sync.Mutex
	modParamCache      string
)

func getModParam(c *gc.C) string {
	modParamCacheMutex.Lock()
	defer modParamCacheMutex.Unlock()
	if modParamCache != "" {
		return modParamCache
	}

	// Check to see if we are vendored, then use the vendored modules.
	cmd := exec.Command("go", "list", "-mod=vendor", "github.com/juju/juju")
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	_, err := cmd.CombinedOutput()
	if err == nil {
		modParamCache = "-mod=vendor"
	} else {
		// We don't want any tests to ever pull modules, so if we don't have vendored
		// modules, then we expect modules to be in the go module cache.
		modParamCache = "-mod=readonly"
	}

	return modParamCache
}

// CopyOrBuildJujuBins from PATH or build the binaries from source.
// If buildids are present in env for the binary, only ever copy and assert if the
// buildids don't match.
func CopyOrBuildJujuBins(c *gc.C, targetDir string, bins ...string) {
	for _, bin := range bins {
		details, ok := binMap[bin]
		if !ok {
			c.Fatalf("unknown juju binary request %q", bin)
			return
		}

		targetPath := path.Join(targetDir, bin)

		if details.BuildID != "" {
			fullPath, err := exec.LookPath(bin)
			if err == exec.ErrNotFound {
				c.Fatalf("could not find %q binary with build id %q", bin, details.BuildID)
				return
			}
			c.Assert(err, jc.ErrorIsNil)

			cmd := exec.Command("go", "tool", "buildid", fullPath)
			out, err := cmd.CombinedOutput()
			c.Assert(err, jc.ErrorIsNil)
			buildID := strings.TrimSpace(string(out))
			c.Assert(buildID, gc.Equals, details.BuildID)

			// If we can avoid a copy with a hard link, lets do that.
			if err := os.Link(fullPath, targetPath); err == nil {
				continue
			}

			if runtime.GOOS == "windows" {
				// For now just symlink it across.
				err := os.Symlink(fullPath, targetPath)
				c.Assert(err, jc.ErrorIsNil)
			} else {
				// Invoke cp
				cmd = exec.Command("cp", "-f", "-p", "-L", fullPath, targetPath)
				out, err = cmd.CombinedOutput()
				c.Logf("copying %q to %q: %s", fullPath, targetPath, string(out))
				c.Assert(err, jc.ErrorIsNil)
			}
			continue
		}

		cmd := exec.Command("go", "build", "-o", targetPath, getModParam(c), details.Pkg)
		out, err := cmd.CombinedOutput()
		c.Logf("building %q: %s", details.Pkg, string(out))
		c.Assert(err, jc.ErrorIsNil)
	}
}
