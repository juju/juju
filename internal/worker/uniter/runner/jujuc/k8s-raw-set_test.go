// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type rawK8sSpecSetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&rawK8sSpecSetSuite{})

var rawPodSpecYaml = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
`[1:]

var rawK8sSpecSetInitTests = []struct {
	args []string
	err  string
}{
	{[]string{"--file", "file", "extra"}, `unrecognized args: \["extra"\]`},
}

func (s *rawK8sSpecSetSuite) TestRawK8sSpecSetInit(c *gc.C) {
	for i, t := range rawK8sSpecSetInitTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, "k8s-raw-set")
		c.Assert(err, jc.ErrorIsNil)
		cmdtesting.TestInit(c, jujuc.NewJujucCommandWrappedForTest(com), t.args, t.err)
	}
}

func (s *rawK8sSpecSetSuite) TestRawK8sSpecSetNoData(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "k8s-raw-set")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)

	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, nil)
	c.Check(code, gc.Equals, 1)
	c.Assert(bufferString(
		ctx.Stderr), gc.Matches,
		".*no k8s raw spec specified: pipe k8s raw spec to command, or specify a file with --file\n")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
}

func (s *rawK8sSpecSetSuite) TestRawK8sSpecSet(c *gc.C) {
	s.assertRawK8sSpecSet(c, "specfile.yaml")
}

func (s *rawK8sSpecSetSuite) TestRawK8sSpecSetStdIn(c *gc.C) {
	s.assertRawK8sSpecSet(c, "-")
}

func (s *rawK8sSpecSetSuite) TestRawK8sSpecSetWithK8sResource(c *gc.C) {
	s.assertRawK8sSpecSet(c, "specfile.yaml")
}

func (s *rawK8sSpecSetSuite) TestRawK8sSpecSetStdInWithK8sResource(c *gc.C) {
	s.assertRawK8sSpecSet(c, "-")
}

func (s *rawK8sSpecSetSuite) assertRawK8sSpecSet(c *gc.C, filename string) {
	hctx := s.GetHookContext(c, -1, "")
	com, args, ctx := s.initCommand(c, hctx, rawPodSpecYaml, filename)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
	c.Check(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	expectedSpecYaml := rawPodSpecYaml
	c.Assert(hctx.info.RawK8sSpec, gc.Equals, expectedSpecYaml)
}

func (s *rawK8sSpecSetSuite) initCommand(c *gc.C, hctx jujuc.Context, yaml string, filename string) (cmd.Command, []string, *cmd.Context) {
	com, err := jujuc.NewCommand(hctx, "k8s-raw-set")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)

	var args []string
	if filename == "-" {
		ctx.Stdin = bytes.NewBufferString(yaml)
	} else if filename != "" {
		filename = filepath.Join(c.MkDir(), filename)
		err := os.WriteFile(filename, []byte(yaml), 0644)
		c.Assert(err, jc.ErrorIsNil)
		args = append(args, "--file", filename)
	}
	return jujuc.NewJujucCommandWrappedForTest(com), args, ctx
}
