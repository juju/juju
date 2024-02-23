// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/juju/charm"
	"github.com/juju/mgo/v3"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v5"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agentbootstrap"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/jujud-controller/agent/agenttest"
	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/mongo/mongotest"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/juju/keys"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type BootstrapSuite struct {
	testing.BaseSuite
	mgotesting.MgoSuite

	bootstrapParams instancecfg.StateInitializationParams

	dataDir          string
	logDir           string
	mongoOplogSize   string
	fakeEnsureMongo  *agenttest.FakeEnsureMongo
	bootstrapName    string
	initialModelUUID string

	toolsStorage storage.Storage

	bootstrapAgentFunc    BootstrapAgentFunc
	dqliteInitializerFunc agentbootstrap.DqliteInitializerFunc
}

var _ = gc.Suite(&BootstrapSuite{})

func (s *BootstrapSuite) SetUpSuite(c *gc.C) {
	storageDir := c.MkDir()
	restorer := jtesting.PatchValue(&envtools.DefaultBaseURL, storageDir)
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
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&sshGenerateKey, func(name string) (string, string, error) {
		return "private-key", "public-key", nil
	})
	s.PatchValue(&corebase.UbuntuDistroInfo, "/path/notexists")

	s.MgoSuite.SetUpTest(c)
	s.dataDir = c.MkDir()
	s.logDir = c.MkDir()
	s.mongoOplogSize = "1234"
	s.fakeEnsureMongo = agenttest.InstallFakeEnsureMongo(s, s.dataDir)
	s.PatchValue(&initiateMongoServer, s.fakeEnsureMongo.InitiateMongo)
	s.makeTestModel(c)

	// Create fake tools.tar.gz and downloaded-tools.txt.
	current := testing.CurrentVersion()
	toolsDir := filepath.FromSlash(agenttools.SharedToolsDir(s.dataDir, current))
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(toolsDir, "tools.tar.gz"), nil, 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.writeDownloadedTools(c, &tools.Tools{Version: current})

	// Create fake local controller charm.
	controllerCharmPath := filepath.Join(s.dataDir, "charms")
	err = os.MkdirAll(controllerCharmPath, 0755)
	c.Assert(err, jc.ErrorIsNil)
	pathToArchive := testcharms.Repo.CharmArchivePath(controllerCharmPath, "juju-controller")
	err = os.Rename(pathToArchive, filepath.Join(controllerCharmPath, "controller.charm"))
	c.Assert(err, jc.ErrorIsNil)

	s.bootstrapAgentFunc = agentbootstrap.NewAgentBootstrap
	s.dqliteInitializerFunc = bootstrapDqliteWithDummyCloudType
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
	err = os.WriteFile(filepath.Join(toolsDir, "downloaded-tools.txt"), data, 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BootstrapSuite) getSystemState(c *gc.C) (*state.State, func()) {
	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      testing.ControllerTag,
		ControllerModelTag: testing.ModelTag,
		MongoSession:       s.Session,
	})
	c.Assert(err, jc.ErrorIsNil)
	systemState, err := pool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	return systemState, func() { pool.Close() }
}

var testPassword = "my-admin-secret"

func (s *BootstrapSuite) initBootstrapCommand(c *gc.C, jobs []model.MachineJob, args ...string) (machineConf agent.ConfigSetterWriter, cmd *BootstrapCommand, err error) {
	if len(jobs) == 0 {
		// Add default jobs.
		jobs = []model.MachineJob{
			model.JobManageModel,
			model.JobHostUnits,
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
		APIAddresses:      []string{"127.0.0.2:1234"},
		CACert:            testing.CACert,
		Values: map[string]string{
			agent.Namespace:      "foobar",
			agent.MongoOplogSize: s.mongoOplogSize,
		},
	}
	servingInfo := controller.StateServingInfo{
		Cert:         testing.CACert,
		PrivateKey:   testing.CAKey,
		CAPrivateKey: "another key",
		APIPort:      3737,
		StatePort:    mgotesting.MgoServer.Port(),
	}

	machineConf, err = agent.NewStateMachineConfig(agentParams, servingInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = machineConf.Write()
	c.Assert(err, jc.ErrorIsNil)

	cmd = NewBootstrapCommand()
	cmd.BootstrapAgent = s.bootstrapAgentFunc
	cmd.DqliteInitializer = s.dqliteInitializerFunc

	err = cmdtesting.InitCommand(cmd, append([]string{"--data-dir", s.dataDir}, args...))
	return machineConf, cmd, err
}

func (s *BootstrapSuite) TestInitializeModel(c *gc.C) {
	machConf, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(cmdtesting.Context(c))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeEnsureMongo.DataDir, gc.Equals, s.dataDir)
	c.Assert(s.fakeEnsureMongo.InitiateCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.EnsureCount, gc.Equals, 1)
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
	c.Assert(servingInfo, jc.DeepEquals, expectInfo)
	expectDialAddrs := []string{fmt.Sprintf("localhost:%d", expectInfo.StatePort)}
	gotDialAddrs := s.fakeEnsureMongo.InitiateParams.DialInfo.Addrs
	c.Assert(gotDialAddrs, gc.DeepEquals, expectDialAddrs)

	c.Assert(
		s.fakeEnsureMongo.InitiateParams.MemberHostPort,
		gc.Matches,
		fmt.Sprintf("testmodel-0.dns:%d$", expectInfo.StatePort),
	)
	c.Assert(s.fakeEnsureMongo.InitiateParams.User, gc.Equals, "")
	c.Assert(s.fakeEnsureMongo.InitiateParams.Password, gc.Equals, "")

	st, closer := s.getSystemState(c)
	defer closer()
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

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := m.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AuthorizedKeys(), gc.Equals, s.bootstrapParams.ControllerModelConfig.AuthorizedKeys())
}

func (s *BootstrapSuite) TestInitializeModelInvalidOplogSize(c *gc.C) {
	s.mongoOplogSize = "NaN"
	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(cmdtesting.Context(c))
	c.Assert(err, gc.ErrorMatches, `failed to start mongo: invalid oplog size: "NaN"`)
}

func (s *BootstrapSuite) TestInitializeModelToolsNotFound(c *gc.C) {
	// bootstrap with 2.99.1 but there will be no tools so version will be reset.
	cfg, err := s.bootstrapParams.ControllerModelConfig.Apply(map[string]interface{}{
		"agent-version": "2.99.1",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.bootstrapParams.ControllerModelConfig = cfg
	s.writeBootstrapParamsFile(c)

	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(cmdtesting.Context(c))
	c.Assert(err, jc.ErrorIsNil)

	st, closer := s.getSystemState(c)
	defer closer()

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	cfg, err = m.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	vers, ok := cfg.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Assert(vers.String(), gc.Equals, "2.99.0")
}

func (s *BootstrapSuite) TestSetConstraints(c *gc.C) {
	s.bootstrapParams.BootstrapMachineConstraints = constraints.Value{Mem: uint64p(4096), CpuCores: uint64p(4)}
	s.bootstrapParams.ModelConstraints = constraints.Value{Mem: uint64p(2048), CpuCores: uint64p(2)}
	s.writeBootstrapParamsFile(c)

	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(cmdtesting.Context(c))
	c.Assert(err, jc.ErrorIsNil)

	st, closer := s.getSystemState(c)
	defer closer()

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
	err = cmd.Run(cmdtesting.Context(c))
	c.Assert(err, jc.ErrorIsNil)

	st, closer := s.getSystemState(c)
	defer closer()
	m, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Jobs(), gc.DeepEquals, expectedJobs)
}

func (s *BootstrapSuite) TestInitialPassword(c *gc.C) {
	machineConf, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = cmd.Run(cmdtesting.Context(c))
	c.Assert(err, jc.ErrorIsNil)

	// Check we can log in to mongo as admin.
	info := mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:      []string{mgotesting.MgoServer.Addr()},
			CACert:     testing.CACert,
			DisableTLS: !mgotesting.MgoServer.SSLEnabled(),
		},
		Tag:      nil, // admin user
		Password: testPassword,
	}
	session, err := mongo.DialWithInfo(info, mongotest.DialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	// We're running Mongo with --noauth; let's explicitly verify
	// that we can login as that user. Even with --noauth, an
	// explicit Login will still be verified.
	adminDB := session.DB("admin")
	err = adminDB.Login("admin", "invalid-password")
	c.Assert(err, gc.ErrorMatches, "(auth|(.*Authentication)) fail(s|ed)\\.?")
	err = adminDB.Login("admin", info.Password)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the admin user has been given an appropriate password
	st, closer := s.getSystemState(c)
	defer closer()
	u, err := st.User(names.NewLocalUserTag("admin"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.PasswordValid(testPassword), jc.IsTrue)

	// Check that the machine configuration has been given a new
	// password and that we can connect to mongo as that machine
	// and that the in-mongo password also verifies correctly.
	machineConf1, err := agent.ReadConfig(agent.ConfigPath(machineConf.DataDir(), names.NewMachineTag("0")))
	c.Assert(err, jc.ErrorIsNil)

	machineMongoInfo, ok := machineConf1.MongoInfo()
	c.Assert(ok, jc.IsTrue)
	session, err = mongo.DialWithInfo(*machineMongoInfo, mongotest.DialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	st, closer = s.getSystemState(c)
	defer closer()

	node, err := st.ControllerNode("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node.HasVote(), jc.IsTrue)
}

func (s *BootstrapSuite) TestInitializeStateArgs(c *gc.C) {
	var called int
	s.bootstrapAgentFunc = func(args agentbootstrap.AgentBootstrapArgs) (*agentbootstrap.AgentBootstrap, error) {
		called++
		c.Assert(args.MongoDialOpts.Direct, jc.IsTrue)
		c.Assert(args.MongoDialOpts.Timeout, gc.Equals, 30*time.Second)
		c.Assert(args.MongoDialOpts.SocketTimeout, gc.Equals, 123*time.Second)
		c.Assert(args.StateInitializationParams.InitialModelConfig, jc.DeepEquals, map[string]interface{}{
			"name": "my-model",
			"uuid": s.initialModelUUID,
		})
		return nil, errors.New("failed to initialize state")
	}

	_, cmd, err := s.initBootstrapCommand(c, nil, "--timeout", "123s")
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(cmdtesting.Context(c))
	c.Assert(err, gc.ErrorMatches, "failed to initialize state")
	c.Assert(called, gc.Equals, 1)
}

func (s *BootstrapSuite) TestInitializeStateMinSocketTimeout(c *gc.C) {
	var called int
	s.bootstrapAgentFunc = func(args agentbootstrap.AgentBootstrapArgs) (*agentbootstrap.AgentBootstrap, error) {
		called++
		c.Assert(args.MongoDialOpts.Direct, jc.IsTrue)
		c.Assert(args.MongoDialOpts.SocketTimeout, gc.Equals, 1*time.Minute)
		return nil, errors.New("failed to initialize state")
	}
	_, cmd, err := s.initBootstrapCommand(c, nil, "--timeout", "13s")
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(cmdtesting.Context(c))
	c.Assert(err, gc.ErrorMatches, "failed to initialize state")
	c.Assert(called, gc.Equals, 1)
}

func (s *BootstrapSuite) TestSystemIdentityWritten(c *gc.C) {
	_, err := os.Stat(filepath.Join(s.dataDir, agent.SystemIdentity))
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(cmdtesting.Context(c))
	c.Assert(err, jc.ErrorIsNil)

	data, err := os.ReadFile(filepath.Join(s.dataDir, agent.SystemIdentity))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "private-key")
}

func createImageMetadata() []*imagemetadata.ImageMetadata {
	return []*imagemetadata.ImageMetadata{{
		Id:         "imageId",
		Storage:    "rootStore",
		VirtType:   "virtType",
		Arch:       "amd64",
		Version:    "22.04",
		Endpoint:   "endpoint",
		RegionName: "region",
	}}
}

func (s *BootstrapSuite) assertWrittenToState(c *gc.C, session *mgo.Session, metadata cloudimagemetadata.Metadata) {
	st, closer := s.getSystemState(c)
	defer closer()

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
		metadata.Source: {metadata},
	})
}

func (s *BootstrapSuite) TestStructuredImageMetadataStored(c *gc.C) {
	s.bootstrapParams.CustomImageMetadata = createImageMetadata()
	s.writeBootstrapParamsFile(c)
	_, cmd, err := s.initBootstrapCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(cmdtesting.Context(c))
	c.Assert(err, jc.ErrorIsNil)

	// This metadata should have also been written to state...
	expect := cloudimagemetadata.Metadata{
		MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Region:          "region",
			Arch:            "amd64",
			Version:         "22.04",
			RootStorageType: "rootStore",
			VirtType:        "virtType",
			Source:          "custom",
		},
		Priority: simplestreams.CUSTOM_CLOUD_DATA,
		ImageId:  "imageId",
	}
	s.assertWrittenToState(c, s.Session, expect)
}

func (s *BootstrapSuite) makeTestModel(c *gc.C) {
	attrs := testing.FakeConfig().Merge(
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
	env, err := environs.Open(context.Background(), provider, environs.OpenParams{
		Cloud:  testing.FakeCloudSpec(),
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = env.PrepareForBootstrap(nullContext(), "controller-1")
	c.Assert(err, jc.ErrorIsNil)

	callCtx := envcontext.WithoutCredentialInvalidator(context.Background())
	s.AddCleanup(func(c *gc.C) {
		err := env.DestroyController(callCtx, controllerCfg.ControllerUUID())
		c.Assert(err, jc.ErrorIsNil)
	})

	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	envtesting.UploadFakeTools(c, s.toolsStorage, cfg.AgentStream(), cfg.AgentStream())
	inst, _, _, err := jujutesting.StartInstance(env, callCtx, testing.FakeControllerConfig().ControllerUUID(), "0")
	c.Assert(err, jc.ErrorIsNil)

	addresses, err := inst.Addresses(callCtx)
	c.Assert(err, jc.ErrorIsNil)
	addr, _ := addresses.OneMatchingScope(network.ScopeMatchPublic)
	s.bootstrapName = addr.Value
	s.initialModelUUID = uuid.MustNewUUID().String()

	var args instancecfg.StateInitializationParams
	args.ControllerConfig = controllerCfg
	args.BootstrapMachineInstanceId = inst.Id()
	args.ControllerModelConfig = env.Config()
	hw := instance.MustParseHardware("arch=amd64 mem=8G")
	args.BootstrapMachineHardwareCharacteristics = &hw
	args.InitialModelConfig = map[string]interface{}{
		"name": "my-model",
		"uuid": s.initialModelUUID,
	}
	args.ControllerCloud = cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
	}
	args.ControllerCharmChannel = charm.Channel{Track: "3.0", Risk: "beta"}
	s.bootstrapParams = args
	s.writeBootstrapParamsFile(c)
}

func (s *BootstrapSuite) writeBootstrapParamsFile(c *gc.C) {
	data, err := s.bootstrapParams.Marshal()
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(s.dataDir, "bootstrap-params"), data, 0600)
	c.Assert(err, jc.ErrorIsNil)
}

func nullContext() environs.BootstrapContext {
	ctx, _ := cmd.DefaultContext()
	ctx.Stdin = io.LimitReader(nil, 0)
	ctx.Stdout = io.Discard
	ctx.Stderr = io.Discard
	return environscmd.BootstrapContext(context.Background(), ctx)
}

func bootstrapDqliteWithDummyCloudType(
	ctx context.Context,
	mgr database.BootstrapNodeManager,
	logger database.Logger,
	concerns ...database.BootstrapConcern,
) error {
	// The dummy cloud type needs to be inserted before the other operations.
	concerns = append([]database.BootstrapConcern{
		database.BootstrapControllerInitConcern(database.EmptyInit, jujutesting.InsertDummyCloudType),
	}, concerns...)

	return database.BootstrapDqlite(ctx, mgr, logger, concerns...)
}
