// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker/peergrouper"
)

var _ = configstore.Default

// We don't want to use JujuConnSuite because it gives us
// an already-bootstrapped environment.
type BootstrapSuite struct {
	testing.BaseSuite
	testing.MgoSuite
	envcfg          string
	instanceId      instance.Id
	dataDir         string
	logDir          string
	fakeEnsureMongo fakeEnsure
	bootstrapName   string
}

var _ = gc.Suite(&BootstrapSuite{})

type fakeEnsure struct {
	ensureCount    int
	initiateCount  int
	dataDir        string
	namespace      string
	info           params.StateServingInfo
	initiateParams peergrouper.InitiateMongoParams
	err            error
}

func (f *fakeEnsure) fakeEnsureMongo(dataDir, namespace string, info params.StateServingInfo) error {
	f.ensureCount++
	f.dataDir, f.namespace, f.info = dataDir, namespace, info
	return f.err
}

func (f *fakeEnsure) fakeInitiateMongo(p peergrouper.InitiateMongoParams) error {
	f.initiateCount++
	f.initiateParams = p
	return nil
}

func (s *BootstrapSuite) SetUpSuite(c *gc.C) {
	s.PatchValue(&ensureMongoServer, s.fakeEnsureMongo.fakeEnsureMongo)
	s.PatchValue(&maybeInitiateMongoServer, s.fakeEnsureMongo.fakeInitiateMongo)

	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	s.makeTestEnv(c)
}

func (s *BootstrapSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
	dummy.Reset()
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.dataDir = c.MkDir()
	s.logDir = c.MkDir()
	s.fakeEnsureMongo = fakeEnsure{}
}

func (s *BootstrapSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

var testPassword = "my-admin-secret"

func testPasswordHash() string {
	return utils.UserPasswordHash(testPassword, utils.CompatSalt)
}

func (s *BootstrapSuite) initBootstrapCommand(c *gc.C, jobs []params.MachineJob, args ...string) (machineConf agent.ConfigSetterWriter, cmd *BootstrapCommand, err error) {
	if len(jobs) == 0 {
		// Add default jobs.
		jobs = []params.MachineJob{
			params.JobManageEnviron, params.JobHostUnits,
		}
	}
	// NOTE: the old test used an equivalent of the NewAgentConfig, but it
	// really should be using NewStateMachineConfig.
	agentParams := agent.AgentConfigParams{
		LogDir:            s.logDir,
		DataDir:           s.dataDir,
		Jobs:              jobs,
		Tag:               "machine-0",
		UpgradedToVersion: version.Current.Number,
		Password:          testPasswordHash(),
		Nonce:             state.BootstrapNonce,
		StateAddresses:    []string{testing.MgoServer.Addr()},
		APIAddresses:      []string{"0.1.2.3:1234"},
		CACert:            testing.CACert,
		Values:            map[string]string{agent.Namespace: "foobar"},
	}
	servingInfo := params.StateServingInfo{
		Cert:       "some cert",
		PrivateKey: "some key",
		APIPort:    3737,
		StatePort:  testing.MgoServer.Port(),
	}

	machineConf, err = agent.NewStateMachineConfig(agentParams, servingInfo)
	c.Assert(err, gc.IsNil)
	err = machineConf.Write()
	c.Assert(err, gc.IsNil)

	cmd = &BootstrapCommand{}

	err = testing.InitCommand(cmd, append([]string{"--data-dir", s.dataDir}, args...))
	return machineConf, cmd, err
}

func (s *BootstrapSuite) TestInitializeEnvironment(c *gc.C) {
	hw := instance.MustParseHardware("arch=amd64 mem=8G")
	machConf, cmd, err := s.initBootstrapCommand(c, nil, "--env-config", s.envcfg, "--instance-id", string(s.instanceId), "--hardware", hw.String())
	c.Assert(err, gc.IsNil)
	err = cmd.Run(nil)
	c.Assert(err, gc.IsNil)

	c.Assert(s.fakeEnsureMongo.dataDir, gc.Equals, s.dataDir)
	c.Assert(s.fakeEnsureMongo.initiateCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.ensureCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.dataDir, gc.Equals, s.dataDir)

	expectInfo, exists := machConf.StateServingInfo()
	c.Assert(exists, jc.IsTrue)
	c.Assert(expectInfo.SharedSecret, gc.Equals, "")

	servingInfo := s.fakeEnsureMongo.info
	c.Assert(len(servingInfo.SharedSecret), gc.Not(gc.Equals), 0)
	servingInfo.SharedSecret = ""
	c.Assert(servingInfo, jc.DeepEquals, expectInfo)
	expectDialAddrs := []string{fmt.Sprintf("127.0.0.1:%d", expectInfo.StatePort)}
	gotDialAddrs := s.fakeEnsureMongo.initiateParams.DialInfo.Addrs
	c.Assert(gotDialAddrs, gc.DeepEquals, expectDialAddrs)

	memberHost := fmt.Sprintf("%s:%d", s.bootstrapName, expectInfo.StatePort)
	c.Assert(s.fakeEnsureMongo.initiateParams.MemberHostPort, gc.Equals, memberHost)
	c.Assert(s.fakeEnsureMongo.initiateParams.User, gc.Equals, "")
	c.Assert(s.fakeEnsureMongo.initiateParams.Password, gc.Equals, "")

	st, err := state.Open(&state.Info{
		Addrs:    []string{testing.MgoServer.Addr()},
		CACert:   testing.CACert,
		Password: testPasswordHash(),
	}, state.DefaultDialOpts(), environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	defer st.Close()
	machines, err := st.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 1)

	instid, err := machines[0].InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(instid, gc.Equals, instance.Id(string(s.instanceId)))

	stateHw, err := machines[0].HardwareCharacteristics()
	c.Assert(err, gc.IsNil)
	c.Assert(stateHw, gc.NotNil)
	c.Assert(*stateHw, gc.DeepEquals, hw)

	cons, err := st.EnvironConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)
}

func (s *BootstrapSuite) TestSetConstraints(c *gc.C) {
	tcons := constraints.Value{Mem: uint64p(2048), CpuCores: uint64p(2)}
	_, cmd, err := s.initBootstrapCommand(c, nil,
		"--env-config", s.envcfg,
		"--instance-id", string(s.instanceId),
		"--constraints", tcons.String(),
	)
	c.Assert(err, gc.IsNil)
	err = cmd.Run(nil)
	c.Assert(err, gc.IsNil)

	st, err := state.Open(&state.Info{
		Addrs:    []string{testing.MgoServer.Addr()},
		CACert:   testing.CACert,
		Password: testPasswordHash(),
	}, state.DefaultDialOpts(), environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	defer st.Close()
	cons, err := st.EnvironConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, tcons)

	machines, err := st.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 1)
	cons, err = machines[0].Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, tcons)
}

func uint64p(v uint64) *uint64 {
	return &v
}

func (s *BootstrapSuite) TestDefaultMachineJobs(c *gc.C) {
	expectedJobs := []state.MachineJob{
		state.JobManageEnviron, state.JobHostUnits,
	}
	_, cmd, err := s.initBootstrapCommand(c, nil, "--env-config", s.envcfg, "--instance-id", string(s.instanceId))
	c.Assert(err, gc.IsNil)
	err = cmd.Run(nil)
	c.Assert(err, gc.IsNil)

	st, err := state.Open(&state.Info{
		Addrs:    []string{testing.MgoServer.Addr()},
		CACert:   testing.CACert,
		Password: testPasswordHash(),
	}, state.DefaultDialOpts(), environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	defer st.Close()
	m, err := st.Machine("0")
	c.Assert(err, gc.IsNil)
	c.Assert(m.Jobs(), gc.DeepEquals, expectedJobs)
}

func (s *BootstrapSuite) TestConfiguredMachineJobs(c *gc.C) {
	jobs := []params.MachineJob{params.JobManageEnviron}
	_, cmd, err := s.initBootstrapCommand(c, jobs, "--env-config", s.envcfg, "--instance-id", string(s.instanceId))
	c.Assert(err, gc.IsNil)
	err = cmd.Run(nil)
	c.Assert(err, gc.IsNil)

	st, err := state.Open(&state.Info{
		Addrs:    []string{testing.MgoServer.Addr()},
		CACert:   testing.CACert,
		Password: testPasswordHash(),
	}, state.DefaultDialOpts(), environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	defer st.Close()
	m, err := st.Machine("0")
	c.Assert(err, gc.IsNil)
	c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{state.JobManageEnviron})
}

func testOpenState(c *gc.C, info *state.Info, expectErrType error) {
	st, err := state.Open(info, state.DefaultDialOpts(), environs.NewStatePolicy())
	if st != nil {
		st.Close()
	}
	if expectErrType != nil {
		c.Assert(err, gc.FitsTypeOf, expectErrType)
	} else {
		c.Assert(err, gc.IsNil)
	}
}

func (s *BootstrapSuite) TestInitialPassword(c *gc.C) {
	machineConf, cmd, err := s.initBootstrapCommand(c, nil, "--env-config", s.envcfg, "--instance-id", string(s.instanceId))
	c.Assert(err, gc.IsNil)

	err = cmd.Run(nil)
	c.Assert(err, gc.IsNil)

	// Check that we cannot now connect to the state without a
	// password.
	info := &state.Info{
		Addrs:  []string{testing.MgoServer.Addr()},
		CACert: testing.CACert,
	}
	testOpenState(c, info, errors.Unauthorizedf(""))

	// Check we can log in to mongo as admin.
	info.Tag, info.Password = "", testPasswordHash()
	st, err := state.Open(info, state.DefaultDialOpts(), environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	// Reset password so the tests can continue to use the same server.
	defer st.Close()
	defer st.SetAdminMongoPassword("")

	// Check that the admin user has been given an appropriate
	// password
	u, err := st.User("admin")
	c.Assert(err, gc.IsNil)
	c.Assert(u.PasswordValid(testPassword), gc.Equals, true)

	// Check that the machine configuration has been given a new
	// password and that we can connect to mongo as that machine
	// and that the in-mongo password also verifies correctly.
	machineConf1, err := agent.ReadConfig(agent.ConfigPath(machineConf.DataDir(), "machine-0"))
	c.Assert(err, gc.IsNil)

	stateinfo, ok := machineConf1.StateInfo()
	c.Assert(ok, jc.IsTrue)
	st, err = state.Open(stateinfo, state.DialOpts{}, environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	defer st.Close()

	m, err := st.Machine("0")
	c.Assert(err, gc.IsNil)
	c.Assert(m.HasVote(), jc.IsTrue)
}

var bootstrapArgTests = []struct {
	input              []string
	err                string
	expectedInstanceId string
	expectedHardware   instance.HardwareCharacteristics
	expectedConfig     map[string]interface{}
}{
	{
		// no value supplied for env-config
		err: "--env-config option must be set",
	}, {
		// empty env-config
		input: []string{"--env-config", ""},
		err:   "--env-config option must be set",
	}, {
		// wrong, should be base64
		input: []string{"--env-config", "name: banana\n"},
		err:   ".*illegal base64 data at input byte.*",
	}, {
		// no value supplied for instance-id
		input: []string{
			"--env-config", base64.StdEncoding.EncodeToString([]byte("name: banana\n")),
		},
		err: "--instance-id option must be set",
	}, {
		// empty instance-id
		input: []string{
			"--env-config", base64.StdEncoding.EncodeToString([]byte("name: banana\n")),
			"--instance-id", "",
		},
		err: "--instance-id option must be set",
	}, {
		input: []string{
			"--env-config", base64.StdEncoding.EncodeToString([]byte("name: banana\n")),
			"--instance-id", "anything",
		},
		expectedInstanceId: "anything",
		expectedConfig:     map[string]interface{}{"name": "banana"},
	}, {
		input: []string{
			"--env-config", base64.StdEncoding.EncodeToString([]byte("name: banana\n")),
			"--instance-id", "anything",
			"--hardware", "nonsense",
		},
		err: `invalid value "nonsense" for flag --hardware: malformed characteristic "nonsense"`,
	}, {
		input: []string{
			"--env-config", base64.StdEncoding.EncodeToString([]byte("name: banana\n")),
			"--instance-id", "anything",
			"--hardware", "arch=amd64 cpu-cores=4 root-disk=2T",
		},
		expectedInstanceId: "anything",
		expectedHardware:   instance.MustParseHardware("arch=amd64 cpu-cores=4 root-disk=2T"),
		expectedConfig:     map[string]interface{}{"name": "banana"},
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
			c.Assert(err, gc.IsNil)
			c.Assert(cmd.EnvConfig, gc.DeepEquals, t.expectedConfig)
			c.Assert(cmd.InstanceId, gc.Equals, t.expectedInstanceId)
			c.Assert(cmd.Hardware, gc.DeepEquals, t.expectedHardware)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
	}
}

func (s *BootstrapSuite) makeTestEnv(c *gc.C) {
	attrs := dummy.SampleConfig().Merge(
		testing.Attrs{
			"agent-version": version.Current.Number.String(),
		},
	).Delete("admin-secret", "ca-private-key")

	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	provider, err := environs.Provider(cfg.Type())
	c.Assert(err, gc.IsNil)
	env, err := provider.Prepare(nullContext(), cfg)
	c.Assert(err, gc.IsNil)

	envtesting.MustUploadFakeTools(env.Storage())
	inst, _, _, err := jujutesting.StartInstance(env, "0")
	c.Assert(err, gc.IsNil)
	s.instanceId = inst.Id()
	s.bootstrapName, err = inst.DNSName()
	c.Assert(err, gc.IsNil)
	s.envcfg = b64yaml(env.Config().AllAttrs()).encode()
}

func nullContext() *cmd.Context {
	ctx, _ := cmd.DefaultContext()
	ctx.Stdin = io.LimitReader(nil, 0)
	ctx.Stdout = ioutil.Discard
	ctx.Stderr = ioutil.Discard
	return ctx
}

type b64yaml map[string]interface{}

func (m b64yaml) encode() string {
	data, err := goyaml.Marshal(m)
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(data)
}
