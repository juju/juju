package juju_test

import (
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/environs/dummy"
	"launchpad.net/juju/go/juju"
	"launchpad.net/juju/go/version"
	"os/exec"
	"path/filepath"
)

type ToolsSuite struct{}

var envs *environs.Environs

func init() {
	var err error
	envs, err = environs.ReadEnvironsBytes([]byte(`
environments:
    foo:
        type: dummy
        zookeeper: false
`))
	if err != nil {
		panic(err)
	}
}

func (ToolsSuite) TearDownTest(c *C) {
	dummy.Reset(nil)
}

var _ = Suite(&ToolsSuite{})

var tools = []struct {
	args   []string
	output string
}{{
	[]string{"juju", "arble"},
	`usage: juju (.|\n)*`,
}, {
	[]string{"jujud", "arble"},
	`usage: jujud (.|\n)*`,
}}

func (ToolsSuite) TestUploadTools(c *C) {
	opc := make(chan dummy.Operation)
	dummy.Reset(opc)
	env, err := envs.Open("")
	c.Assert(err, IsNil)

	conn := &juju.Conn{env}
	errc := make(chan error, 1)
	go func() {
		errc <- conn.UploadTools()
	}()
	op := <-opc
	c.Assert(op.Kind, Equals, dummy.OpUploadTools)
	c.Assert(op.Version, Equals, version.Current)

	// Unarchive the tool executables from the reader
	// that's been given to the dummy environment
	// UploadTools calls.
	dir := c.MkDir()
	unarchive(c, dir, op.Upload)
	dummy.Reset(nil)
	c.Assert((<-opc).Kind, Equals, dummy.OpNone)
	c.Assert(<-errc, IsNil)

	// Verify that each tool executes and produces some
	// characteristic output.
	for _, t := range tools {
		out, _ := exec.Command(
			filepath.Join(dir, t.args[0]),
			t.args[1:]...).CombinedOutput()
		c.Check(string(out), Matches, t.output)
	}
}

func (ToolsSuite) TestUploadBadBuild(c *C) {
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
