package environs_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/environs/dummy"
	"launchpad.net/juju/go/version"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ToolsSuite struct{
	env environs.Environ
}

func (t *ToolsSuite) SetUpTest(c *C) {
	env, err := environs.NewEnviron(map[string]interface{}{
		"name": "test",
		"type": "dummy",
		"zookeeper": false,
	})
	c.Assert(err, IsNil)
	t.env = env
}

func (t *ToolsSuite) TearDownTest(c *C) {
	dummy.Reset()
}

var envs *environs.Environs

var currentToolsPath = toolsPath(version.Current.String(), environs.CurrentSeries, environs.CurrentArch)

func mkVersion(vers string) version.Version {
	v, err := version.Parse(vers)
	if err != nil {
		panic(err)
	}
	return v
}

func toolsPathForVersion(v version.Version, series, arch string) string {
	return fmt.Sprintf("tools/juju-%v-%s-%s.tgz", v, series, arch)
}
	
func toolsPath(vers, os, arch string) string {
	return toolsPathForVersion(mkVersion(vers), os, arch)
}

var _ = Suite(&ToolsSuite{})

func (t *ToolsSuite) TestPutTools(c *C) {
	err := environs.PutTools(t.env.Storage())
	c.Assert(err, IsNil)

	for i, getTools := range []func(environs.Environ, string) error{
		environs.GetTools,
		getToolsWithTar,
	} {
		c.Logf("test %d", i)
		// Unarchive the tool executables into a temp directory.
		dir := c.MkDir()
		err = getTools(t.env, dir)
		c.Assert(err, IsNil)

		// Verify that each tool executes and produces some
		// characteristic output.
		for _, tool := range []string{"juju", "jujud"} {
			out, err := exec.Command(filepath.Join(dir, tool), "arble").CombinedOutput()
			if err != nil {
				c.Assert(err, FitsTypeOf, (*exec.ExitError)(nil))
			}
			c.Check(string(out), Matches, fmt.Sprintf(`usage: %s (.|\n)*`, tool))
		}
	}
}

// getToolsWithTar is the same as GetTools but uses tar
// itself so we're not just testing the Go tar package against
// itself.
func getToolsWithTar(env environs.Environ, dir string) error {
	// TODO search the store for the right tools.
	r, err := env.Storage().Get(currentToolsPath)
	if err != nil {
		return err
	}
	defer r.Close()

	// unarchive using actual tar command so we're
	// not just verifying the Go tar package against itself.
	cmd := exec.Command("tar", "xz")
	cmd.Dir = dir
	cmd.Stdin = r
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar extract failed: %s", out)
	}
	return nil
}

// Test that the upload procedure fails correctly
// when the build process fails (because of a bad Go source
// file in this case).
func (t *ToolsSuite) TestUploadBadBuild(c *C) {
	gopath := c.MkDir()
	pkgdir := filepath.Join(gopath, "src", "launchpad.net", "juju", "go", "cmd", "broken")
	err := os.MkdirAll(pkgdir, 0777)
	c.Assert(err, IsNil)

	err = ioutil.WriteFile(filepath.Join(pkgdir, "broken.go"), []byte("nope"), 0666)
	c.Assert(err, IsNil)

	defer os.Setenv("GOPATH", os.Getenv("GOPATH"))
	os.Setenv("GOPATH", gopath)

	err = environs.PutTools(t.env.Storage())
	c.Assert(err, ErrorMatches, `build failed: exit status 1; can't load package:(.|\n)*`)
}

type toolsSpec struct {
	version string
	os      string
	arch    string
}

var findToolsTests = []struct {
	version    version.Version
	contents []string
	publicContents []string
	expect   string
	expectPublic bool
	err      string
}{{
	// current version should be satisfied by current tools path.
	version: version.Current,
	contents: []string{currentToolsPath},
	expect: currentToolsPath,
}, {
	// major versions don't match.
	version: mkVersion("1.0.0"),
	contents: []string{
		toolsPath("0.0.9", environs.CurrentSeries, environs.CurrentArch),
	},
	err: "no compatible tools found",
}, {
	// major versions don't match.
	version: mkVersion("1.0.0"),
	contents: []string{
		toolsPath("2.0.9", environs.CurrentSeries, environs.CurrentArch),
	},
	err: "no compatible tools found",
}, {
	// fall back to public storage when nothing found in private.
	version: mkVersion("1.0.0"),
	contents: []string{
		toolsPath("0.0.9", environs.CurrentSeries, environs.CurrentArch),
	},
	publicContents: []string{
		toolsPath("1.0.0", environs.CurrentSeries, environs.CurrentArch),
	},
	expect: "public-" + toolsPath("1.0.0", environs.CurrentSeries, environs.CurrentArch),
}, {
	// always use private storage in preference to public storage.
	version: mkVersion("1.0.0"),
	contents: []string{
		toolsPath("1.0.2", environs.CurrentSeries, environs.CurrentArch),
	},
	publicContents: []string{
		toolsPath("1.0.9", environs.CurrentSeries, environs.CurrentArch),
	},
	expect: toolsPath("1.0.2", environs.CurrentSeries, environs.CurrentArch),
}, {
	// we'll use an earlier version if the major version number matches.
	version: mkVersion("1.99.99"),
	contents: []string{
		toolsPath("1.0.0", environs.CurrentSeries, environs.CurrentArch),
	},
	expect: toolsPath("1.0.0", environs.CurrentSeries, environs.CurrentArch),
}, {
	// check that version comparing is numeric, not alphabetical.
	version: mkVersion("1.0.0"),
	contents: []string{
		toolsPath("1.0.9", environs.CurrentSeries, environs.CurrentArch),
		toolsPath("1.0.10", environs.CurrentSeries, environs.CurrentArch),
		toolsPath("1.0.11", environs.CurrentSeries, environs.CurrentArch),
	},
	expect: toolsPath("1.0.11", environs.CurrentSeries, environs.CurrentArch),
}, {
	// minor version wins over patch version.
	version: mkVersion("1.0.0"),
	contents: []string{
		toolsPath("1.9.11", environs.CurrentSeries, environs.CurrentArch),
		toolsPath("1.10.10", environs.CurrentSeries, environs.CurrentArch),
		toolsPath("1.11.9", environs.CurrentSeries, environs.CurrentArch),
	},
	expect: toolsPath("1.11.9", environs.CurrentSeries, environs.CurrentArch),
}, {
	// mismatching series or architecture is ignored.
	version: mkVersion("1.0.0"),
	contents: []string{
		toolsPath("1.9.9", "foo", environs.CurrentArch),
		toolsPath("1.9.9", environs.CurrentSeries, "foo"),
		toolsPath("1.0.0", environs.CurrentSeries, environs.CurrentArch),
	},
	expect: toolsPath("1.0.0", environs.CurrentSeries, environs.CurrentArch),
},
}

func (t *ToolsSuite) TestFindTools(c *C) {
	originalVersion := version.Current
	defer func() {
		version.Current = originalVersion
	}()

	for i, tt := range findToolsTests {
		c.Logf("test %d", i)
		version.Current = tt.version
		for _, name := range tt.contents {
			err := t.env.Storage().Put(name, strings.NewReader(name), int64(len(name)))
			c.Assert(err, IsNil)
		}
		// The contents of all files in the public storage is prefixed with "public-" so
		// that we can easily tell if we've got the right thing.
		for _, name := range tt.publicContents {
			data := "public-" + name
			err := t.env.PublicStorage().(environs.Storage).Put(name, strings.NewReader(data), int64(len(data)))
			c.Assert(err, IsNil)
		}
		url, err := environs.FindTools(t.env)
		if tt.err != "" {
			c.Assert(err, ErrorMatches, tt.err)
		} else {
			c.Assert(err, IsNil)
			resp, err := http.Get(url)
			c.Assert(err, IsNil)
			data, err := ioutil.ReadAll(resp.Body)
			c.Assert(err, IsNil)
			c.Assert(string(data), Equals, tt.expect, Commentf("url %s", url))
		}
		t.env.Destroy(nil)
	}
}
