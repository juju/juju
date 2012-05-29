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

type ToolsSuite struct {
	env environs.Environ
}

func (t *ToolsSuite) SetUpTest(c *C) {
	env, err := environs.NewEnviron(map[string]interface{}{
		"name":      "test",
		"type":      "dummy",
		"zookeeper": false,
	})
	c.Assert(err, IsNil)
	t.env = env
}

func (t *ToolsSuite) TearDownTest(c *C) {
	dummy.Reset()
}

var envs *environs.Environs

var currentToolsPath = mkToolsPath(version.Current.String())

func mkVersion(vers string) version.Version {
	v, err := version.Parse(vers)
	if err != nil {
		panic(err)
	}
	return v
}

func mkToolsPath(vers string) string {
	return environs.ToolsPath(mkVersion(vers), environs.CurrentSeries, environs.CurrentArch)
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
	version        version.Version // version to assume is current for the test.
	contents       []string        // names in private storage.
	publicContents []string        // names in public storage.
	expect         string          // the name we expect to find (if no error).
	err            string          // the error we expect to find (if not blank).
}{{
	// current version should be satisfied by current tools path.
	version:  version.Current,
	contents: []string{currentToolsPath},
	expect:   currentToolsPath,
}, {
	// major versions don't match.
	version: mkVersion("1.0.0"),
	contents: []string{
		mkToolsPath("0.0.9"),
	},
	err: "no compatible tools found",
}, {
	// major versions don't match.
	version: mkVersion("1.0.0"),
	contents: []string{
		mkToolsPath("2.0.9"),
	},
	err: "no compatible tools found",
}, {
	// fall back to public storage when nothing found in private.
	version: mkVersion("1.0.0"),
	contents: []string{
		mkToolsPath("0.0.9"),
	},
	publicContents: []string{
		mkToolsPath("1.0.0"),
	},
	expect: "public-" + mkToolsPath("1.0.0"),
}, {
	// always use private storage in preference to public storage.
	version: mkVersion("1.0.0"),
	contents: []string{
		mkToolsPath("1.0.2"),
	},
	publicContents: []string{
		mkToolsPath("1.0.9"),
	},
	expect: mkToolsPath("1.0.2"),
}, {
	// we'll use an earlier version if the major version number matches.
	version: mkVersion("1.99.99"),
	contents: []string{
		mkToolsPath("1.0.0"),
	},
	expect: mkToolsPath("1.0.0"),
}, {
	// check that version comparing is numeric, not alphabetical.
	version: mkVersion("1.0.0"),
	contents: []string{
		mkToolsPath("1.0.9"),
		mkToolsPath("1.0.10"),
		mkToolsPath("1.0.11"),
	},
	expect: mkToolsPath("1.0.11"),
}, {
	// minor version wins over patch version.
	version: mkVersion("1.0.0"),
	contents: []string{
		mkToolsPath("1.9.11"),
		mkToolsPath("1.10.10"),
		mkToolsPath("1.11.9"),
	},
	expect: mkToolsPath("1.11.9"),
}, {
	// mismatching series or architecture is ignored.
	version: mkVersion("1.0.0"),
	contents: []string{
		environs.ToolsPath(mkVersion("1.9.9"), "foo", environs.CurrentArch),
		environs.ToolsPath(mkVersion("1.9.9"), environs.CurrentSeries, "foo"),
		mkToolsPath("1.0.0"),
	},
	expect: mkToolsPath("1.0.0"),
},
}

func (t *ToolsSuite) TestFindTools(c *C) {
	for i, tt := range findToolsTests {
		c.Logf("test %d", i)
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
		url, err := environs.FindTools(t.env, tt.version, environs.CurrentSeries, environs.CurrentArch)
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

var readSeriesTests = []struct {
	contents string
	series   string
}{{
	`DISTRIB_ID=Ubuntu
DISTRIB_RELEASE=12.04
DISTRIB_CODENAME=precise
DISTRIB_DESCRIPTION="Ubuntu 12.04 LTS"`,
	"precise",
}, {
	"DISTRIB_CODENAME= \tprecise\t",
	"precise",
},  {
	`DISTRIB_CODENAME="precise"`,
	"precise",
},   {
	"DISTRIB_CODENAME='precise'",
	"precise",
}, {
	`DISTRIB_ID=Ubuntu
DISTRIB_RELEASE=12.10
DISTRIB_CODENAME=quantal
DISTRIB_DESCRIPTION="Ubuntu 12.10"`,
	"quantal",
}, {
	"",
	"unknown",
},
}

func (t *ToolsSuite) TestReadSeries(c *C) {
	d := c.MkDir()
	f := filepath.Join(d, "foo")
	for i, t := range readSeriesTests {
		c.Logf("test %d", i)
		err := ioutil.WriteFile(f, []byte(t.contents), 0666)
		c.Assert(err, IsNil)
		c.Assert(environs.ReadSeries(f), Equals, t.series)
	}
}

func (t *ToolsSuite) TestCurrentSeries(c *C) {
	s := environs.CurrentSeries
	if s == "unknown" {
		s = "n/a"
	}
	out, err := exec.Command("lsb_release", "-c").CombinedOutput()
	if err != nil {
		// If the command fails (for instance if we're running on some other
		// platform) then CurrentSeries should be unknown.
		c.Assert(s, Equals, "n/a")
	} else {
		c.Assert(string(out), Equals, "Codename:\t"+s+"\n")
	}
}
