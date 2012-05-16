package environs_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	_ "launchpad.net/juju/go/environs/dummy"
	"os"
	"os/exec"
	"path/filepath"
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

func (ToolsSuite) TestPutTools(c *C) {
	env, err := envs.Open("")
	c.Assert(err, IsNil)

	err = environs.PutTools(env.Storage())
	c.Assert(err, IsNil)

	// Unarchive the tool executables into a temp directory.
	dir := c.MkDir()
	err = environs.GetTools(env.Storage(), dir)
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
