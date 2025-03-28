// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type K8sSpecGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&K8sSpecGetSuite{})

var k8sSpecGetInitTests = []struct {
	args []string
	err  string
}{
	{[]string{"extra"}, `unrecognized args: \["extra"\]`},
}

func (s *K8sSpecGetSuite) TestK8sSpecGetInit(c *gc.C) {
	for i, t := range k8sSpecGetInitTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewHookCommand(hctx, "k8s-spec-get")
		c.Assert(err, jc.ErrorIsNil)
		cmdtesting.TestInit(c, jujuc.NewJujucCommandWrappedForTest(com), t.args, t.err)
	}
}

func (s *K8sSpecGetSuite) TestK8sSpecSet(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	hctx.info.K8sSpec = "k8sspec"
	com, err := jujuc.NewHookCommand(hctx, "k8s-spec-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)

	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, nil)
	c.Check(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "k8sspec")
}
