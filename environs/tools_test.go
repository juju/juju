package environs_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	_ "launchpad.net/juju/go/environs/dummy"
	"launchpad.net/juju/go/version"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type ToolsSuite struct{}

var envs *environs.Environs

var toolsPath = toolsPathForVersion(version.Current, 
TODO
fmt.Sprintf("tools/juju-%v-%s-%s.tgz", version.Current, runtime.GOOS, runtime.GOARCH)

// toolsPathForVersion returns a path for the juju tools with the
// given version, OS and architecture.
func toolsPathForVersion(v version.Version, series, arch string) string {
	return fmt.Sprintf(toolPrefix+"%v-%s-%s.tgz", v, series, arch)
}

const toolsConf = `
environments:
    foo:
        type: dummy
        zookeeper: false
`

func init() {
	var err error
	envs, err = environs.ReadEnvironsBytes([]byte(toolsConf))
	if err != nil {
		panic(err)
	}
}

var _ = Suite(&ToolsSuite{})

func (ToolsSuite) TestPutTools(c *C) {
	env, err := envs.Open("")
	c.Assert(err, IsNil)

	err = environs.PutTools(env.Storage())
	c.Assert(err, IsNil)

	for i, getTools := range []func(environs.StorageReader, string) error{
		environs.GetTools,
		getToolsWithTar,
	} {
		c.Logf("test %d", i)
		// Unarchive the tool executables into a temp directory.
		dir := c.MkDir()
		err = getTools(env.Storage(), dir)
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
func getToolsWithTar(store environs.StorageReader, dir string) error {
	// TODO search the store for the right tools.
	r, err := store.Get(toolsPath)
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
func (ToolsSuite) TestUploadBadBuild(c *C) {
	gopath := c.MkDir()
	pkgdir := filepath.Join(gopath, "src", "launchpad.net", "juju", "go", "cmd", "broken")
	err := os.MkdirAll(pkgdir, 0777)
	c.Assert(err, IsNil)

	err = ioutil.WriteFile(filepath.Join(pkgdir, "broken.go"), []byte("nope"), 0666)
	c.Assert(err, IsNil)

	defer os.Setenv("GOPATH", os.Getenv("GOPATH"))
	os.Setenv("GOPATH", gopath)

	env, err := envs.Open("")
	c.Assert(err, IsNil)

	err = environs.PutTools(env.Storage())
	c.Assert(err, ErrorMatches, `build failed: exit status 1; can't load package:(.|\n)*`)
}

type toolsSpec struct {
	version string
	os      string
	arch    string
}

func toolsPath(vers, os, arch string) string {
	v, err := version.Parse(vers)
	if err != nil {
		panic(err)
	}
	return version.ToolsPathForVersion(v, os, arch)
}

var findToolsTests = []struct {
	major    int
	contents []string
	expect   string
	err      string
}{{
	version.Current.Major,
	[]string{version.ToolsPath},
	version.ToolsPath,
	"",
}, {
	1,
	[]string{
		toolsPath("0.0.9", version.CurrentOS, version.CurrentArch),
	},
	"",
	"no compatible tools found",
}, {
	1,
	[]string{
		toolsPath("2.0.9", version.CurrentOS, version.CurrentArch),
	},
	"",
	"no compatible tools found",
}, {
	1,
	[]string{
		toolsPath("1.0.9", version.CurrentOS, version.CurrentArch),
		toolsPath("1.0.10", version.CurrentOS, version.CurrentArch),
		toolsPath("1.0.11", version.CurrentOS, version.CurrentArch),
	},
	toolsPath("1.0.11", version.CurrentOS, version.CurrentArch),
	"",
}, {
	1,
	[]string{
		toolsPath("1.9.11", version.CurrentOS, version.CurrentArch),
		toolsPath("1.10.10", version.CurrentOS, version.CurrentArch),
		toolsPath("1.11.9", version.CurrentOS, version.CurrentArch),
	},
	toolsPath("1.11.9", version.CurrentOS, version.CurrentArch),
	"",
}, {
	1,
	[]string{
		toolsPath("1.9.9", "foo", version.CurrentArch),
		toolsPath("1.9.9", version.CurrentOS, "foo"),
		toolsPath("1.0.0", version.CurrentOS, version.CurrentArch),
	},
	toolsPath("1.0.0", version.CurrentOS, version.CurrentArch),
	"",
}}

func (t *localServerSuite) TestFindTools(c *C) {
	oldMajorVersion := *ec2.VersionCurrentMajor
	defer func() {
		*ec2.VersionCurrentMajor = oldMajorVersion
	}()
	for i, tt := range findToolsTests {
		c.Logf("test %d", i)
		*ec2.VersionCurrentMajor = tt.major
		for _, name := range tt.contents {
			err := t.env.PutFile(name, strings.NewReader(name), int64(len(name)))
			c.Assert(err, IsNil)
		}
		url, err := ec2.FindTools(t.env)
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
