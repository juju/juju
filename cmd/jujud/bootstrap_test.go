// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agentbootstrap"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/keys"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

// We don't want to use JujuConnSuite because it gives us
// an already-bootstrapped environment.
type BootstrapSuite struct {
	testing.BaseSuite
	gitjujutesting.MgoSuite

	bootstrapParamsFile string
	bootstrapParams     instancecfg.StateInitializationParams

	dataDir         string
	logDir          string
	mongoOplogSize  string
	fakeEnsureMongo *agenttest.FakeEnsureMongo
	bootstrapName   string
	hostedModelUUID string

	toolsStorage storage.Storage
}

var _ = gc.Suite(&BootstrapSuite{})

func (s *BootstrapSuite) SetUpSuite(c *gc.C) {
	storageDir := c.MkDir()
	restorer := gitjujutesting.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	s.toolsStorage = stor

	s.BaseSuite.SetUpSuite(c)
	s.AddCleanup(func(*gc.C) {
		restorer()
	})
	s.MgoSuite.SetUpSuite(c)
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
}

func (s *BootstrapSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
	dummy.Reset(c)
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&sshGenerateKey, func(name string) (string, string, error) {
		return "private-key", "public-key", nil
	})

	s.MgoSuite.SetUpTest(c)
	s.dataDir = c.MkDir()
	s.logDir = c.MkDir()
	s.bootstrapParamsFile = filepath.Join(s.dataDir, "bootstrap-params")
	s.mongoOplogSize = "1234"
	s.fakeEnsureMongo = agenttest.InstallFakeEnsureMongo(s)
	s.PatchValue(&initiateMongoServer, s.fakeEnsureMongo.InitiateMongo)
	s.makeTestModel(c)

	// Create fake tools.tar.gz and downloaded-tools.txt.
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	toolsDir := filepath.FromSlash(agenttools.SharedToolsDir(s.dataDir, current))
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(toolsDir, "tools.tar.gz"), nil, 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.writeDownloadedTools(c, &tools.Tools{Version: current})

	// Create fake gui.tar.bz2 and downloaded-gui.txt.
	guiDir := filepath.FromSlash(agenttools.SharedGUIDir(s.dataDir))
	err = os.MkdirAll(guiDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(guiDir, "gui.tar.bz2"), nil, 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.writeDownloadedGUI(c, &tools.GUIArchive{
		Version: version.MustParse("2.0.42"),
	})
}

func (s *BootstrapSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *BootstrapSuite) writeDownloadedTools(c *gc.C, tools *tools.Tools) {
	toolsDir := filepath.FromSlash(agenttools.SharedToolsDir(s.dataDir, tools.Version))
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	data, err := json.Marshal(tools)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(toolsDir, "downloaded-tools.txt"), data, 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BootstrapSuite) writeDownloadedGUI(c *gc.C, gui *tools.GUIArchive) {
	guiDir := filepath.FromSlash(agenttools.SharedGUIDir(s.dataDir))
	err := os.MkdirAll(guiDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	data, err := json.Marshal(gui)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(guiDir, "downloaded-gui.txt"), data, 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BootstrapSuite) TestGUIArchiveInfoNotFound(c *gc.C) {
	dir := filepath.FromSlash(agenttools.SharedGUIDir(s.dataDir))
	info := filepath.Join(dir, "downloaded-gui.txt")
	err := os.Remove(info)
	c.Assert(err, jc.ErrorIsNil)
	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)

	var tw loggo.TestWriter
	err = loggo.RegisterWriter("bootstrap-test", &tw)
	c.Assert(err, jc.ErrorIsNil)
	defer loggo.RemoveWriter("bootstrap-test")

	err = cmd.Run(nil)
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.WARNING,
		`cannot set up Juju GUI: cannot fetch GUI info: GUI metadata not found`,
	}})
}

func (s *BootstrapSuite) TestGUIArchiveInfoError(c *gc.C) {
	if runtime.GOOS == "windows" {
		// TODO frankban: skipping for now due to chmod problems with mode 0000
		// on Windows. We will re-enable this test after further investigation:
		// "jujud bootstrap" is never run on Windows anyway.
		c.Skip("needs chmod investigation")
	}
	dir := filepath.FromSlash(agenttools.SharedGUIDir(s.dataDir))
	info := filepath.Join(dir, "downloaded-gui.txt")
	err := os.Chmod(info, 0000)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Chmod(info, 0600)
	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)

	var tw loggo.TestWriter
	err = loggo.RegisterWriter("bootstrap-test", &tw)
	c.Assert(err, jc.ErrorIsNil)
	defer loggo.RemoveWriter("bootstrap-test")

	err = cmd.Run(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.WARNING,
		`cannot set up Juju GUI: cannot fetch GUI info: cannot read GUI metadata in tools directory: .*`,
	}})
}

func (s *BootstrapSuite) TestGUIArchiveError(c *gc.C) {
	dir := filepath.FromSlash(agenttools.SharedGUIDir(s.dataDir))
	archive := filepath.Join(dir, "gui.tar.bz2")
	err := os.Remove(archive)
	c.Assert(err, jc.ErrorIsNil)
	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)

	var tw loggo.TestWriter
	err = loggo.RegisterWriter("bootstrap-test", &tw)
	c.Assert(err, jc.ErrorIsNil)
	defer loggo.RemoveWriter("bootstrap-test")

	err = cmd.Run(nil)
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.WARNING,
		`cannot set up Juju GUI: cannot read GUI archive: .*`,
	}})
}

func (s *BootstrapSuite) TestGUIArchiveSuccess(c *gc.C) {
	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)

	var tw loggo.TestWriter
	err = loggo.RegisterWriter("bootstrap-test", &tw)
	c.Assert(err, jc.ErrorIsNil)
	defer loggo.RemoveWriter("bootstrap-test")

	err = cmd.Run(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.DEBUG,
		`Juju GUI successfully set up`,
	}})

	// Retrieve the state so that it is possible to access the GUI storage.
	st, err := state.Open(testing.ModelTag, testing.ControllerTag, &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
		Password: testPassword,
	}, mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	// The GUI archive has been uploaded to the GUI storage.
	storage, err := st.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	allMeta, err := storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allMeta, gc.HasLen, 1)
	c.Assert(allMeta[0].Version, gc.Equals, "2.0.42")

	// The current GUI version has been set.
	vers, err := st.GUIVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers.String(), gc.Equals, "2.0.42")
}

var testPassword = "my-admin-secret"

func (s *BootstrapSuite) initBootstrapCommand(c *gc.C, jobs []multiwatcher.MachineJob, args ...string) (machineConf agent.ConfigSetterWriter, cmd *BootstrapCommand, err error) {
	if len(jobs) == 0 {
		// Add default jobs.
		jobs = []multiwatcher.MachineJob{
			multiwatcher.JobManageModel,
			multiwatcher.JobHostUnits,
		}
	}
	// NOTE: the old test used an equivalent of the NewAgentConfig, but it
	// really should be using NewStateMachineConfig.
	agentParams := agent.AgentConfigParams{
		Paths: agent.Paths{
			LogDir:  s.logDir,
			DataDir: s.dataDir,
		},
		Jobs:              jobs,
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: jujuversion.Current,
		Password:          testPassword,
		Nonce:             agent.BootstrapNonce,
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
		StateAddresses:    []string{gitjujutesting.MgoServer.Addr()},
		APIAddresses:      []string{"0.1.2.3:1234"},
		CACert:            testing.CACert,
		Values: map[string]string{
			agent.Namespace:      "foobar",
			agent.MongoOplogSize: s.mongoOplogSize,
		},
	}
	servingInfo := params.StateServingInfo{
		Cert:         "some cert",
		PrivateKey:   "some key",
		CAPrivateKey: "another key",
		APIPort:      3737,
		StatePort:    gitjujutesting.MgoServer.Port(),
	}

	machineConf, err = agent.NewStateMachineConfig(agentParams, servingInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = machineConf.Write()
	c.Assert(err, jc.ErrorIsNil)

	if len(args) == 0 {
		args = []string{s.bootstrapParamsFile}
	}
	cmd = NewBootstrapCommand()
	err = testing.InitCommand(cmd, append([]string{"--data-dir", s.dataDir}, args...))
	return machineConf, cmd, err
}

func (s *BootstrapSuite) TestInitializeEnvironment(c *gc.C) {
	machConf, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeEnsureMongo.DataDir, gc.Equals, s.dataDir)
	c.Assert(s.fakeEnsureMongo.InitiateCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.EnsureCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.DataDir, gc.Equals, s.dataDir)
	c.Assert(s.fakeEnsureMongo.OplogSize, gc.Equals, 1234)

	expectInfo, exists := machConf.StateServingInfo()
	c.Assert(exists, jc.IsTrue)
	c.Assert(expectInfo.SharedSecret, gc.Equals, "")
	c.Assert(expectInfo.SystemIdentity, gc.Equals, "")

	servingInfo := s.fakeEnsureMongo.Info
	c.Assert(len(servingInfo.SharedSecret), gc.Not(gc.Equals), 0)
	c.Assert(len(servingInfo.SystemIdentity), gc.Not(gc.Equals), 0)
	servingInfo.SharedSecret = ""
	servingInfo.SystemIdentity = ""
	expect := cmdutil.ParamsStateServingInfoToStateStateServingInfo(expectInfo)
	c.Assert(servingInfo, jc.DeepEquals, expect)
	expectDialAddrs := []string{fmt.Sprintf("127.0.0.1:%d", expectInfo.StatePort)}
	gotDialAddrs := s.fakeEnsureMongo.InitiateParams.DialInfo.Addrs
	c.Assert(gotDialAddrs, gc.DeepEquals, expectDialAddrs)

	c.Assert(s.fakeEnsureMongo.InitiateParams.MemberHostPort, gc.Equals, expectDialAddrs[0])
	c.Assert(s.fakeEnsureMongo.InitiateParams.User, gc.Equals, "")
	c.Assert(s.fakeEnsureMongo.InitiateParams.Password, gc.Equals, "")

	st, err := state.Open(testing.ModelTag, testing.ControllerTag, &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
		Password: testPassword,
	}, mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	machines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)

	instid, err := machines[0].InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instid, gc.Equals, s.bootstrapParams.BootstrapMachineInstanceId)

	stateHw, err := machines[0].HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateHw, gc.NotNil)
	c.Assert(stateHw, gc.DeepEquals, s.bootstrapParams.BootstrapMachineHardwareCharacteristics)

	cons, err := st.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)

	cfg, err := st.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AuthorizedKeys(), gc.Equals, s.bootstrapParams.ControllerModelConfig.AuthorizedKeys()+"\npublic-key")
}

func (s *BootstrapSuite) TestInitializeEnvironmentInvalidOplogSize(c *gc.C) {
	s.mongoOplogSize = "NaN"
	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, gc.ErrorMatches, `failed to start mongo: invalid oplog size: "NaN"`)
}

func (s *BootstrapSuite) TestInitializeEnvironmentToolsNotFound(c *gc.C) {
	// bootstrap with 1.99.1 but there will be no tools so version will be reset.
	cfg, err := s.bootstrapParams.ControllerModelConfig.Apply(map[string]interface{}{
		"agent-version": "1.99.1",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.bootstrapParams.ControllerModelConfig = cfg
	s.writeBootstrapParamsFile(c)

	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	st, err := state.Open(testing.ModelTag, testing.ControllerTag, &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
		Password: testPassword,
	}, mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	cfg, err = st.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	vers, ok := cfg.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Assert(vers.String(), gc.Equals, "1.99.0")
}

func (s *BootstrapSuite) TestSetConstraints(c *gc.C) {
	s.bootstrapParams.BootstrapMachineConstraints = constraints.Value{Mem: uint64p(4096), CpuCores: uint64p(4)}
	s.bootstrapParams.ModelConstraints = constraints.Value{Mem: uint64p(2048), CpuCores: uint64p(2)}
	s.writeBootstrapParamsFile(c)

	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	st, err := state.Open(testing.ModelTag, testing.ControllerTag, &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
		Password: testPassword,
	}, mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	cons, err := st.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, s.bootstrapParams.ModelConstraints)

	machines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	cons, err = machines[0].Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, s.bootstrapParams.BootstrapMachineConstraints)
}

func uint64p(v uint64) *uint64 {
	return &v
}

func (s *BootstrapSuite) TestDefaultMachineJobs(c *gc.C) {
	expectedJobs := []state.MachineJob{
		state.JobManageModel,
		state.JobHostUnits,
	}
	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	st, err := state.Open(testing.ModelTag, testing.ControllerTag, &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
		Password: testPassword,
	}, mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	m, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Jobs(), gc.DeepEquals, expectedJobs)
}

func (s *BootstrapSuite) TestConfiguredMachineJobs(c *gc.C) {
	jobs := []multiwatcher.MachineJob{multiwatcher.JobManageModel}
	_, cmd, err := s.initBootstrapCommand(c, jobs)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	st, err := state.Open(testing.ModelTag, testing.ControllerTag, &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
		Password: testPassword,
	}, mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	m, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{state.JobManageModel})
}

func (s *BootstrapSuite) TestInitialPassword(c *gc.C) {
	machineConf, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = cmd.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	info := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
	}

	// Check we can log in to mongo as admin.
	info.Tag, info.Password = nil, testPassword
	st, err := state.Open(testing.ModelTag, testing.ControllerTag, info, mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	// We're running Mongo with --noauth; let's explicitly verify
	// that we can login as that user. Even with --noauth, an
	// explicit Login will still be verified.
	adminDB := st.MongoSession().DB("admin")
	err = adminDB.Login("admin", "invalid-password")
	c.Assert(err, gc.ErrorMatches, "(auth|(.*Authentication)) fail(s|ed)\\.?")
	err = adminDB.Login("admin", info.Password)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the admin user has been given an appropriate
	// password
	u, err := st.User(names.NewLocalUserTag("admin"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.PasswordValid(testPassword), jc.IsTrue)

	// Check that the machine configuration has been given a new
	// password and that we can connect to mongo as that machine
	// and that the in-mongo password also verifies correctly.
	machineConf1, err := agent.ReadConfig(agent.ConfigPath(machineConf.DataDir(), names.NewMachineTag("0")))
	c.Assert(err, jc.ErrorIsNil)

	stateinfo, ok := machineConf1.MongoInfo()
	c.Assert(ok, jc.IsTrue)
	st, err = state.Open(testing.ModelTag, testing.ControllerTag, stateinfo, mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	m, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.HasVote(), jc.IsTrue)
}

var bootstrapArgTests = []struct {
	input                       []string
	err                         string
	expectedBootstrapParamsFile string
}{
	{
		err:   "bootstrap-params file must be specified",
		input: []string{"--data-dir", "/tmp/juju/data/dir"},
	}, {
		input: []string{"/some/where"},
		expectedBootstrapParamsFile: "/some/where",
	},
}

func (s *BootstrapSuite) TestBootstrapArgs(c *gc.C) {
	for i, t := range bootstrapArgTests {
		c.Logf("test %d", i)
		var args []string
		args = append(args, t.input...)
		_, cmd, err := s.initBootstrapCommand(c, nil, args...)
		if t.err == "" {
			c.Assert(cmd, gc.NotNil)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(cmd.BootstrapParamsFile, gc.Equals, t.expectedBootstrapParamsFile)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
	}
}

func (s *BootstrapSuite) TestInitializeStateArgs(c *gc.C) {
	var called int
	initializeState := func(_ names.UserTag, _ agent.ConfigSetter, args agentbootstrap.InitializeStateParams, dialOpts mongo.DialOpts, _ state.NewPolicyFunc) (_ *state.State, _ *state.Machine, resultErr error) {
		called++
		c.Assert(dialOpts.Direct, jc.IsTrue)
		c.Assert(dialOpts.Timeout, gc.Equals, 30*time.Second)
		c.Assert(dialOpts.SocketTimeout, gc.Equals, 123*time.Second)
		c.Assert(args.HostedModelConfig, jc.DeepEquals, map[string]interface{}{
			"name": "hosted-model",
			"uuid": s.hostedModelUUID,
		})
		return nil, nil, errors.New("failed to initialize state")
	}
	s.PatchValue(&agentInitializeState, initializeState)
	_, cmd, err := s.initBootstrapCommand(c, nil, "--timeout", "123s", s.bootstrapParamsFile)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, gc.ErrorMatches, "failed to initialize state")
	c.Assert(called, gc.Equals, 1)
}

func (s *BootstrapSuite) TestInitializeStateMinSocketTimeout(c *gc.C) {
	var called int
	initializeState := func(_ names.UserTag, _ agent.ConfigSetter, _ agentbootstrap.InitializeStateParams, dialOpts mongo.DialOpts, _ state.NewPolicyFunc) (_ *state.State, _ *state.Machine, resultErr error) {
		called++
		c.Assert(dialOpts.Direct, jc.IsTrue)
		c.Assert(dialOpts.SocketTimeout, gc.Equals, 1*time.Minute)
		return nil, nil, errors.New("failed to initialize state")
	}

	s.PatchValue(&agentInitializeState, initializeState)
	_, cmd, err := s.initBootstrapCommand(c, nil, "--timeout", "13s", s.bootstrapParamsFile)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, gc.ErrorMatches, "failed to initialize state")
	c.Assert(called, gc.Equals, 1)
}

func (s *BootstrapSuite) TestSystemIdentityWritten(c *gc.C) {
	_, err := os.Stat(filepath.Join(s.dataDir, agent.SystemIdentity))
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	data, err := ioutil.ReadFile(filepath.Join(s.dataDir, agent.SystemIdentity))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "private-key")
}

func (s *BootstrapSuite) TestDownloadedToolsMetadata(c *gc.C) {
	// Tools downloaded by cloud-init script.
	s.testToolsMetadata(c, false)
}

func (s *BootstrapSuite) TestUploadedToolsMetadata(c *gc.C) {
	// Tools uploaded over ssh.
	s.writeDownloadedTools(c, &tools.Tools{
		Version: version.Binary{
			Number: jujuversion.Current,
			Arch:   arch.HostArch(),
			Series: series.HostSeries(),
		},
		URL: "file:///does/not/matter",
	})
	s.testToolsMetadata(c, true)
}

func (s *BootstrapSuite) testToolsMetadata(c *gc.C, exploded bool) {
	envtesting.RemoveFakeToolsMetadata(c, s.toolsStorage)

	_, cmd, err := s.initBootstrapCommand(c, nil)

	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	// We don't write metadata at bootstrap anymore.
	simplestreamsMetadata, err := envtools.ReadMetadata(s.toolsStorage, "released")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(simplestreamsMetadata, gc.HasLen, 0)

	// The tools should have been added to tools storage, and
	// exploded into each of the supported series of
	// the same operating system if the tools were uploaded.
	st, err := state.Open(testing.ModelTag, testing.ControllerTag, &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
		Password: testPassword,
	}, mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	expectedSeries := make(set.Strings)
	if exploded {
		for _, ser := range series.SupportedSeries() {
			os, err := series.GetOSFromSeries(ser)
			c.Assert(err, jc.ErrorIsNil)
			hostos, err := series.GetOSFromSeries(series.HostSeries())
			c.Assert(err, jc.ErrorIsNil)
			if os == hostos {
				expectedSeries.Add(ser)
			}
		}
	} else {
		expectedSeries.Add(series.HostSeries())
	}

	storage, err := st.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	metadata, err := storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, expectedSeries.Size())
	for _, m := range metadata {
		v := version.MustParseBinary(m.Version)
		c.Assert(expectedSeries.Contains(v.Series), jc.IsTrue)
	}
}

func createImageMetadata() []*imagemetadata.ImageMetadata {
	return []*imagemetadata.ImageMetadata{{
		Id:         "imageId",
		Storage:    "rootStore",
		VirtType:   "virtType",
		Arch:       "amd64",
		Version:    "14.04",
		Endpoint:   "endpoint",
		RegionName: "region",
	}}
}

func assertWrittenToState(c *gc.C, metadata cloudimagemetadata.Metadata) {
	st, err := state.Open(testing.ModelTag, testing.ControllerTag, &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
		Password: testPassword,
	}, mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	// find all image metadata in state
	all, err := st.CloudImageMetadataStorage.FindMetadata(cloudimagemetadata.MetadataFilter{})
	c.Assert(err, jc.ErrorIsNil)
	// if there was no stream, it should have defaulted to "released"
	if metadata.Stream == "" {
		metadata.Stream = "released"
	}
	if metadata.DateCreated == 0 && len(all[metadata.Source]) > 0 {
		metadata.DateCreated = all[metadata.Source][0].DateCreated
	}
	c.Assert(all, gc.DeepEquals, map[string][]cloudimagemetadata.Metadata{
		metadata.Source: []cloudimagemetadata.Metadata{metadata},
	})
}

func (s *BootstrapSuite) TestStructuredImageMetadataStored(c *gc.C) {
	s.bootstrapParams.CustomImageMetadata = createImageMetadata()
	s.writeBootstrapParamsFile(c)
	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

	// This metadata should have also been written to state...
	expect := cloudimagemetadata.Metadata{
		MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Region:          "region",
			Arch:            "amd64",
			Version:         "14.04",
			Series:          "trusty",
			RootStorageType: "rootStore",
			VirtType:        "virtType",
			Source:          "custom",
		},
		Priority: simplestreams.CUSTOM_CLOUD_DATA,
		ImageId:  "imageId",
	}
	assertWrittenToState(c, expect)
}

func (s *BootstrapSuite) TestStructuredImageMetadataInvalidSeries(c *gc.C) {
	s.bootstrapParams.CustomImageMetadata = createImageMetadata()
	s.bootstrapParams.CustomImageMetadata[0].Version = "woat"
	s.writeBootstrapParamsFile(c)

	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(nil)
	c.Assert(err, gc.ErrorMatches, `cannot determine series for version woat: unknown series for version: \"woat\"`)
}

func (s *BootstrapSuite) makeTestModel(c *gc.C) {
	attrs := dummy.SampleConfig().Merge(
		testing.Attrs{
			"agent-version": jujuversion.Current.String(),
		},
	).Delete("admin-secret", "ca-private-key")
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	provider, err := environs.Provider(cfg.Type())
	c.Assert(err, jc.ErrorIsNil)
	controllerCfg := testing.FakeControllerConfig()
	cfg, err = provider.PrepareConfig(environs.PrepareConfigParams{
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	env, err := provider.Open(environs.OpenParams{
		Cloud:  dummy.SampleCloudSpec(),
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = env.PrepareForBootstrap(nullContext())
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		err := env.DestroyController(controllerCfg.ControllerUUID())
		c.Assert(err, jc.ErrorIsNil)
	})

	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	envtesting.MustUploadFakeTools(s.toolsStorage, cfg.AgentStream(), cfg.AgentStream())
	inst, _, _, err := jujutesting.StartInstance(env, testing.FakeControllerConfig().ControllerUUID(), "0")
	c.Assert(err, jc.ErrorIsNil)

	addresses, err := inst.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	addr, _ := network.SelectPublicAddress(addresses)
	s.bootstrapName = addr.Value
	s.hostedModelUUID = utils.MustNewUUID().String()

	var args instancecfg.StateInitializationParams
	args.ControllerConfig = controllerCfg
	args.BootstrapMachineInstanceId = inst.Id()
	args.ControllerModelConfig = env.Config()
	hw := instance.MustParseHardware("arch=amd64 mem=8G")
	args.BootstrapMachineHardwareCharacteristics = &hw
	args.HostedModelConfig = map[string]interface{}{
		"name": "hosted-model",
		"uuid": s.hostedModelUUID,
	}
	args.ControllerCloudName = "dummy"
	args.ControllerCloud = cloud.Cloud{
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
	}
	s.bootstrapParams = args
	s.writeBootstrapParamsFile(c)
}

func (s *BootstrapSuite) writeBootstrapParamsFile(c *gc.C) {
	data, err := s.bootstrapParams.Marshal()
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(s.bootstrapParamsFile, data, 0600)
	c.Assert(err, jc.ErrorIsNil)
}

func nullContext() environs.BootstrapContext {
	ctx, _ := cmd.DefaultContext()
	ctx.Stdin = io.LimitReader(nil, 0)
	ctx.Stdout = ioutil.Discard
	ctx.Stderr = ioutil.Discard
	return modelcmd.BootstrapContext(ctx)
}

type b64yaml map[string]interface{}

func (m b64yaml) encode() string {
	data, err := goyaml.Marshal(m)
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(data)
}
