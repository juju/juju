package reboot

import (
	"errors"
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/apiserver/params"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { gc.TestingT(t) }

type RebootSuite struct {
	// jujutesting.JujuConnSuite

	// acfg    agent.Config
	// mgoInst testing.MgoInstance
	// st      api.Connection

	// tmpDir           string
	// rebootScriptName string
}

var _ = gc.Suite(&RebootSuite{})

func (s *RebootSuite) TestExecuteReboot_ShouldDoNothingReturns(c *gc.C) {
	// If this doesn't return immediately, it would panic
	ExecuteReboot(nil, nil, 0, params.ShouldDoNothing, nil)
}

func (s *RebootSuite) TestExecuteReboot_RebootStateFail(c *gc.C) {
	openRebootState := func() (reboot.State, error) {
		return nil, errors.New("foo")
	}
	err := ExecuteReboot(nil, openRebootState, 0, params.ShouldReboot, nil)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "foo")
}

func (s *RebootSuite) TestExecuteReboot_RebootOKReturnsErr(c *gc.C) {
	openRebootState := func() (reboot.State, error) {
		return nil, nil
	}
	rebootOK := make(chan error)
	go func() { rebootOK <- errors.New("foo") }()
	err := ExecuteReboot(nil, openRebootState, 0, params.ShouldReboot, rebootOK)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "foo")
}

func (s *RebootSuite) TestExecuteReboot_ExecuteBuiltShutdownCmd(c *gc.C) {
	openRebootState := func() (reboot.State, error) {
		return nil, nil
	}
	rebootOK := make(chan error)
	go func() { rebootOK <- errors.New("foo") }()
	err := ExecuteReboot(nil, openRebootState, 0, params.ShouldReboot, rebootOK)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "foo")
}

// func (s *RebootSuite) SetUpTest(c *gc.C) {
// 	if testing.GOVERSION < 1.3 {
// 		c.Skip("skipping test, lxd requires Go 1.3 or later")
// 	}

// 	s.JujuConnSuite.SetUpTest(c)
// 	testing.PatchExecutableAsEchoArgs(c, s, rebootBin)
// 	s.PatchEnvironment("TEMP", c.MkDir())

// 	s.tmpDir = c.MkDir()
// 	s.rebootScriptName = "juju-reboot-script"
// 	s.PatchValue(tmpFile, func() (*os.File, error) {
// 		script := s.rebootScript(c)
// 		return os.Create(script)
// 	})

// 	s.mgoInst.EnableAuth = true
// 	err := s.mgoInst.Start(coretesting.Certs)
// 	c.Assert(err, jc.ErrorIsNil)

// 	configParams := agent.AgentConfigParams{
// 		Paths:             agent.Paths{DataDir: c.MkDir()},
// 		Tag:               names.NewMachineTag("0"),
// 		UpgradedToVersion: jujuversion.Current,
// 		StateAddresses:    []string{s.mgoInst.Addr()},
// 		CACert:            coretesting.CACert,
// 		Password:          "fake",
// 		Model:             s.State.ModelTag(),
// 		MongoVersion:      mongo.Mongo24,
// 	}
// 	s.st, _ = s.OpenAPIAsNewMachine(c)

// 	s.acfg, err = agent.NewAgentConfig(configParams)
// 	c.Assert(err, jc.ErrorIsNil)
// }

// func (s *RebootSuite) TearDownTest(c *gc.C) {
// 	s.mgoInst.Destroy()
// 	s.JujuConnSuite.TearDownTest(c)
// }

// func (s *RebootSuite) rebootScript(c *gc.C) string {
// 	return filepath.Join(s.tmpDir, s.rebootScriptName)
// }
