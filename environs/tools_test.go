package environs_test

import (
	"fmt"
	"io"
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

func (ToolsSuite) TestUploadTools(c *C) {
	env, err := envs.Open("")
	c.Assert(err, IsNil)

	err = environs.UploadTools(env)
	c.Assert(err, IsNil)

	name := fmt.Sprintf("tools/juju-%v-%s-%s.tgz", version.Current, runtime.GOOS, runtime.GOARCH)
	r, err := env.GetFile(name)
	c.Assert(err, IsNil)

	// Unarchive the tool executables into a temp directory.
	dir := c.MkDir()
	unarchive(c, dir, r)

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

	err = environs.UploadTools(env)
	c.Assert(err, ErrorMatches, `build failed: exit status 1; can't load package:(.|\n)*`)
}

func unarchive(c *C, dir string, r io.ReadCloser) {
	defer r.Close()

	// unarchive using actual tar command so we're
	// not just verifying the Go tar package against itself.
	cmd := exec.Command("tar", "xz")
	cmd.Dir = dir
	cmd.Stdin = r
	out, err := cmd.CombinedOutput()
	if err != nil {
		c.Logf("%s", out)
		c.Fatalf("tar xz failed: %v", err)
	}
}
