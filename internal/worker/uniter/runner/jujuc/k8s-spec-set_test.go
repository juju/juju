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

type K8sSpecSetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&K8sSpecSetSuite{})

var (
	podSpecYaml = `
podSpec:
  foo: bar
`[1:]

	k8sResourcesYaml = `
kubernetesResources:
  pod:
    restartPolicy: OnFailure
    activeDeadlineSeconds: 10
    terminationGracePeriodSeconds: 20
    securityContext:
      runAsNonRoot: true
      supplementalGroups: [1,2]
    priority: 30
    readinessGates:
      - conditionType: PodScheduled
    dnsPolicy: ClusterFirstWithHostNet
  secrets:
    - name: build-robot-secret
      annotations:
          kubernetes.io/service-account.name: build-robot
      type: kubernetes.io/service-account-token
      stringData:
          config.yaml: |-
              apiUrl: "https://my.api.com/api/v1"
              username: fred
              password: shhhh
`[1:]
)

var k8sSpecSetInitTests = []struct {
	args []string
	err  string
}{
	{[]string{"--file", "file", "extra"}, `unrecognized args: \["extra"\]`},
}

func (s *K8sSpecSetSuite) TestK8sSpecSetInit(c *gc.C) {
	for i, t := range k8sSpecSetInitTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewHookCommand(hctx, "k8s-spec-set")
		c.Assert(err, jc.ErrorIsNil)
		cmdtesting.TestInit(c, jujuc.NewJujucCommandWrappedForTest(com), t.args, t.err)
	}
}

func (s *K8sSpecSetSuite) TestK8sSpecSetNoData(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewHookCommand(hctx, "k8s-spec-set")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)

	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, nil)
	c.Check(code, gc.Equals, 1)
	c.Assert(bufferString(
		ctx.Stderr), gc.Matches,
		".*no k8s spec specified: pipe k8s spec to command, or specify a file with --file\n")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
}

func (s *K8sSpecSetSuite) TestK8sSpecSet(c *gc.C) {
	s.assertK8sSpecSet(c, "specfile.yaml", false)
}

func (s *K8sSpecSetSuite) TestK8sSpecSetStdIn(c *gc.C) {
	s.assertK8sSpecSet(c, "-", false)
}

func (s *K8sSpecSetSuite) TestK8sSpecSetWithK8sResource(c *gc.C) {
	s.assertK8sSpecSet(c, "specfile.yaml", true)
}

func (s *K8sSpecSetSuite) TestK8sSpecSetStdInWithK8sResource(c *gc.C) {
	s.assertK8sSpecSet(c, "-", true)
}

func (s *K8sSpecSetSuite) assertK8sSpecSet(c *gc.C, filename string, withK8sResource bool) {
	hctx := s.GetHookContext(c, -1, "")
	com, args, ctx := s.initCommand(c, hctx, podSpecYaml, filename, withK8sResource)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
	c.Check(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	expectedSpecYaml := podSpecYaml
	if withK8sResource {
		expectedSpecYaml += k8sResourcesYaml
	}
	c.Assert(hctx.info.K8sSpec, gc.Equals, expectedSpecYaml)
}

func (s *K8sSpecSetSuite) initCommand(
	c *gc.C, hctx jujuc.Context, yaml string, filename string, withK8sResource bool,
) (cmd.Command, []string, *cmd.Context) {
	com, err := jujuc.NewHookCommand(hctx, "k8s-spec-set")
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
	if withK8sResource {
		k8sResourceFileName := "k8sresources.yaml"
		k8sResourceFileName = filepath.Join(c.MkDir(), k8sResourceFileName)
		err := os.WriteFile(k8sResourceFileName, []byte(k8sResourcesYaml), 0644)
		c.Assert(err, jc.ErrorIsNil)
		args = append(args, "--k8s-resources", k8sResourceFileName)
	}
	return jujuc.NewJujucCommandWrappedForTest(com), args, ctx
}
