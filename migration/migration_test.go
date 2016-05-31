// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/state/storage"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type ImportSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&ImportSuite{})

func (s *ImportSuite) SetUpTest(c *gc.C) {
	// Specify the config to use for the controller model before calling
	// SetUpTest of the StateSuite, otherwise we get testing.ModelConfig(c).
	// The default provider type specified in the testing.ModelConfig function
	// is one that isn't registered as a valid provider. For our tests here we
	// need a real registered provider, so we use the dummy provider.
	// NOTE: make a better test provider.
	env, err := environs.Prepare(
		modelcmd.BootstrapContext(testing.Context(c)),
		jujuclienttesting.NewMemStore(),
		environs.PrepareParams{
			ControllerName: "dummycontroller",
			BaseConfig:     dummy.SampleConfig(),
			CloudName:      "dummy",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.InitialConfig = testing.CustomModelConfig(c, env.Config().AllAttrs())
	s.StateSuite.SetUpTest(c)
}

func (s *ImportSuite) TestBadBytes(c *gc.C) {
	bytes := []byte("not a model")
	model, st, err := migration.ImportModel(s.State, bytes)
	c.Check(st, gc.IsNil)
	c.Check(model, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "yaml: unmarshal errors:\n.*")
}

func (s *ImportSuite) TestImportModel(c *gc.C) {
	model, err := s.State.Export()
	c.Check(err, jc.ErrorIsNil)

	controllerConfig, err := s.State.ModelConfig()
	c.Check(err, jc.ErrorIsNil)

	// Update the config values in the exported model for different values for
	// "state-port", "api-port", and "ca-cert". Also give the model a new UUID
	// and name so we can import it nicely.
	model.UpdateConfig(map[string]interface{}{
		"name":       "new-model",
		"uuid":       utils.MustNewUUID().String(),
		"state-port": 12345,
		"api-port":   54321,
		"ca-cert":    "not really a cert",
	})

	bytes, err := description.Serialize(model)
	c.Check(err, jc.ErrorIsNil)

	dbModel, dbState, err := migration.ImportModel(s.State, bytes)
	c.Check(err, jc.ErrorIsNil)
	defer dbState.Close()

	dbConfig, err := dbModel.Config()
	c.Assert(err, jc.ErrorIsNil)
	attrs := dbConfig.AllAttrs()
	c.Assert(attrs["state-port"], gc.Equals, controllerConfig.StatePort())
	c.Assert(attrs["api-port"], gc.Equals, controllerConfig.APIPort())
	cacert, ok := controllerConfig.CACert()
	c.Assert(ok, jc.IsTrue)
	c.Assert(attrs["ca-cert"], gc.Equals, cacert)
	c.Assert(attrs["controller-uuid"], gc.Equals, controllerConfig.UUID())
}

func (s *ImportSuite) TestUploadBinariesTools(c *gc.C) {
	// Create a model that has three different tools versions:
	// one for a machine, one for a container, and one for a unit agent.
	// We don't care about the actual validity of the model (it isn't).
	model := description.NewModel(description.ModelArgs{
		Owner: names.NewUserTag("me"),
	})
	machine := model.AddMachine(description.MachineArgs{
		Id: names.NewMachineTag("0"),
	})
	machine.SetTools(description.AgentToolsArgs{
		Version: version.MustParseBinary("2.0.1-trusty-amd64"),
	})
	container := machine.AddContainer(description.MachineArgs{
		Id: names.NewMachineTag("0/lxc/0"),
	})
	container.SetTools(description.AgentToolsArgs{
		Version: version.MustParseBinary("2.0.5-trusty-amd64"),
	})
	service := model.AddService(description.ServiceArgs{
		Tag:      names.NewApplicationTag("magic"),
		CharmURL: "local:trusty/magic",
	})
	unit := service.AddUnit(description.UnitArgs{
		Tag: names.NewUnitTag("magic/0"),
	})
	unit.SetTools(description.AgentToolsArgs{
		Version: version.MustParseBinary("2.0.3-trusty-amd64"),
	})

	uploader := &fakeUploader{tools: make(map[version.Binary]string)}
	config := migration.UploadBinariesConfig{
		State:            &fakeStateStorage{},
		Model:            model,
		Target:           &fakeAPIConnection{},
		GetCharmUploader: func(api.Connection) migration.CharmUploader { return &noOpUploader{} },
		GetToolsUploader: func(target api.Connection) migration.ToolsUploader {
			return uploader
		},
		GetStateStorage:     func(migration.UploadBackend) storage.Storage { return &fakeCharmsStorage{} },
		GetCharmStoragePath: func(migration.UploadBackend, *charm.URL) (string, error) { return "", nil },
	}
	err := migration.UploadBinaries(config)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(uploader.tools, jc.DeepEquals, map[version.Binary]string{
		version.MustParseBinary("2.0.1-trusty-amd64"): "fake tools 2.0.1-trusty-amd64",
		version.MustParseBinary("2.0.3-trusty-amd64"): "fake tools 2.0.3-trusty-amd64",
		version.MustParseBinary("2.0.5-trusty-amd64"): "fake tools 2.0.5-trusty-amd64",
	})
}

func (s *ImportSuite) TestStreamCharmsTools(c *gc.C) {
	model := description.NewModel(description.ModelArgs{
		Owner: names.NewUserTag("me"),
	})
	model.AddService(description.ServiceArgs{
		Tag:      names.NewApplicationTag("magic"),
		CharmURL: "local:trusty/magic",
	})
	model.AddService(description.ServiceArgs{
		Tag:      names.NewApplicationTag("magic"),
		CharmURL: "cs:trusty/postgresql-42",
	})

	uploader := &fakeUploader{charms: make(map[string]string)}
	config := migration.UploadBinariesConfig{
		State:            &fakeStateStorage{},
		Model:            model,
		Target:           &fakeAPIConnection{},
		GetCharmUploader: func(api.Connection) migration.CharmUploader { return uploader },
		GetToolsUploader: func(target api.Connection) migration.ToolsUploader { return &noOpUploader{} },
		GetStateStorage:  func(migration.UploadBackend) storage.Storage { return &fakeCharmsStorage{} },
		GetCharmStoragePath: func(_ migration.UploadBackend, u *charm.URL) (string, error) {
			return "/path/for/" + u.String(), nil
		},
	}
	err := migration.UploadBinaries(config)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(uploader.charms, jc.DeepEquals, map[string]string{
		"local:trusty/magic":      "fake file at /path/for/local:trusty/magic",
		"cs:trusty/postgresql-42": "fake file at /path/for/cs:trusty/postgresql-42",
	})
}

type fakeStateStorage struct {
	tools  fakeToolsStorage
	charms fakeCharmsStorage
}

type fakeCharmsStorage struct {
	storage.Storage
}

type fakeAPIConnection struct {
	api.Connection
}

type fakeToolsStorage struct {
	binarystorage.Storage
	closed bool
}

func (f *fakeStateStorage) ToolsStorage() (binarystorage.StorageCloser, error) {
	return &f.tools, nil
}

func (f *fakeStateStorage) ModelUUID() string {
	return testing.ModelTag.Id()
}

func (f *fakeStateStorage) MongoSession() *mgo.Session {
	return nil
}

func (f *fakeStateStorage) Charm(*charm.URL) (*state.Charm, error) {
	return nil, nil
}

func (f *fakeToolsStorage) Open(v string) (binarystorage.Metadata, io.ReadCloser, error) {
	buff := bytes.NewBufferString(fmt.Sprintf("fake tools %s", v))
	return binarystorage.Metadata{}, ioutil.NopCloser(buff), nil
}

func (f *fakeToolsStorage) Close() error {
	f.closed = true
	return nil
}

func (f *fakeCharmsStorage) Get(path string) (io.ReadCloser, int64, error) {
	buff := bytes.NewBufferString(fmt.Sprintf("fake file at %s", path))
	return ioutil.NopCloser(buff), int64(buff.Len()), nil
}

type fakeUploader struct {
	tools  map[version.Binary]string
	charms map[string]string
}

func (f *fakeUploader) UploadTools(r io.ReadSeeker, v version.Binary, _ ...string) (tools.List, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, errors.Trace(err)
	}

	f.tools[v] = string(data)

	uploaded := &tools.Tools{
		Version: v,
	}
	return tools.List{uploaded}, nil
}

func (f *fakeUploader) UploadCharm(u *charm.URL, r io.ReadSeeker) (*charm.URL, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, errors.Trace(err)
	}

	f.charms[u.String()] = string(data)
	return u, nil
}

type noOpUploader struct{}

func (*noOpUploader) UploadCharm(*charm.URL, io.ReadSeeker) (*charm.URL, error) {
	return nil, nil
}

func (*noOpUploader) UploadTools(io.ReadSeeker, version.Binary, ...string) (tools.List, error) {
	return nil, nil
}

type ExportSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&ExportSuite{})

func (s *ExportSuite) TestExportModel(c *gc.C) {
	bytes, err := migration.ExportModel(s.State)
	c.Assert(err, jc.ErrorIsNil)
	// The bytes must be a valid model.
	_, err = description.Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
}

type PrecheckSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&PrecheckSuite{})

// Assert that *state.State implements the PrecheckBackend
var _ migration.PrecheckBackend = (*state.State)(nil)

func (*PrecheckSuite) TestPrecheckCleanups(c *gc.C) {
	backend := &fakePrecheckBackend{}
	err := migration.Precheck(backend)
	c.Assert(err, jc.ErrorIsNil)
}

func (*PrecheckSuite) TestPrecheckCleanupsError(c *gc.C) {
	backend := &fakePrecheckBackend{
		cleanupError: errors.New("boom"),
	}
	err := migration.Precheck(backend)
	c.Assert(err, gc.ErrorMatches, "precheck cleanups: boom")
}

func (*PrecheckSuite) TestPrecheckCleanupsNeeded(c *gc.C) {
	backend := &fakePrecheckBackend{
		cleanupNeeded: true,
	}
	err := migration.Precheck(backend)
	c.Assert(err, gc.ErrorMatches, "precheck failed: cleanup needed")
}

type fakePrecheckBackend struct {
	cleanupNeeded bool
	cleanupError  error
}

func (f *fakePrecheckBackend) NeedsCleanup() (bool, error) {
	return f.cleanupNeeded, f.cleanupError
}

type InternalSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&InternalSuite{})

func (s *InternalSuite) TestControllerValues(c *gc.C) {
	config := testing.ModelConfig(c)
	fields := migration.ControllerValues(config)
	c.Assert(fields, jc.DeepEquals, map[string]interface{}{
		"controller-uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"state-port":      19034,
		"api-port":        17777,
		"ca-cert":         testing.CACert,
	})
}

func (s *InternalSuite) TestUpdateConfigFromProvider(c *gc.C) {
	controllerConfig := testing.ModelConfig(c)
	configAttrs := testing.FakeConfig()
	configAttrs["type"] = "dummy"
	// Fake the "state-id" so the provider thinks it is prepared already.
	configAttrs["state-id"] = "42"
	// We need to specify a valid provider type, so we use dummy.
	// The dummy provider grabs the UUID from the controller config
	// and returns it in the map with the key "controller-uuid", similar
	// to what the azure provider will need to do.
	model := description.NewModel(description.ModelArgs{
		Owner:  names.NewUserTag("test-admin"),
		Config: configAttrs,
	})

	err := migration.UpdateConfigFromProvider(model, controllerConfig)
	c.Assert(err, jc.ErrorIsNil)

	modelConfig := model.Config()
	c.Assert(modelConfig["controller-uuid"], gc.Equals, controllerConfig.UUID())
}

type CharmInternalSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&CharmInternalSuite{})

func (s *CharmInternalSuite) TestCharmStoragePath(c *gc.C) {
	charm := s.Factory.MakeCharm(c, nil)

	path, err := migration.GetCharmStoragePath(s.State, charm.URL())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.Equals, "fake-storage-path")
}
