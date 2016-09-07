// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud/reboot"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type RebootSuite struct {
	jujutesting.JujuConnSuite

	acfg    agent.Config
	mgoInst testing.MgoInstance
	st      api.Connection

	tmpDir           string
	rebootScriptName string
}

var _ = gc.Suite(&RebootSuite{})

func (s *RebootSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)

	// These tests only patch out LXC, so only run full-stack tests
	// over LXC.
	s.PatchValue(&instance.ContainerTypes, []instance.ContainerType{instance.LXD})
}

func (s *RebootSuite) SetUpTest(c *gc.C) {

	s.JujuConnSuite.SetUpTest(c)
	testing.PatchExecutableAsEchoArgs(c, s, rebootBin)
	s.PatchEnvironment("TEMP", c.MkDir())

	s.tmpDir = c.MkDir()
	s.rebootScriptName = "juju-reboot-script"
	s.PatchValue(reboot.TmpFile, func() (*os.File, error) {
		script := s.rebootScript(c)
		return os.Create(script)
	})

	s.mgoInst.EnableAuth = true
	err := s.mgoInst.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)

	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: c.MkDir()},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: jujuversion.Current,
		StateAddresses:    []string{s.mgoInst.Addr()},
		CACert:            coretesting.CACert,
		Password:          "fake",
		Controller:        s.State.ControllerTag(),
		Model:             s.State.ModelTag(),
		MongoVersion:      mongo.Mongo24,
	}
	s.st, _ = s.OpenAPIAsNewMachine(c)

	s.acfg, err = agent.NewAgentConfig(configParams)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RebootSuite) TearDownTest(c *gc.C) {
	s.mgoInst.Destroy()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *RebootSuite) rebootScript(c *gc.C) string {
	return filepath.Join(s.tmpDir, s.rebootScriptName)
}
