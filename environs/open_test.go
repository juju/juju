// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cert"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type OpenSuite struct {
	testing.FakeJujuHomeSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&OpenSuite{})

func (s *OpenSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	testing.WriteEnvironments(c, testing.MultipleEnvConfigNoDefault)
}

func (s *OpenSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.FakeJujuHomeSuite.TearDownTest(c)
}

func (*OpenSuite) TestNewDummyEnviron(c *gc.C) {
	// matches *Settings.Map()
	cfg, err := config.New(config.NoDefaults, dummySampleConfig())
	c.Assert(err, gc.IsNil)
	ctx := testing.Context(c)
	env, err := environs.Prepare(cfg, ctx, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	envtesting.UploadFakeTools(c, env.Storage())
	err = bootstrap.Bootstrap(ctx, env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.IsNil)
}

func (s *OpenSuite) TestUpdateEnvInfo(c *gc.C) {
	store := configstore.NewMem()
	ctx := testing.Context(c)
	_, err := environs.PrepareFromName("erewhemos", ctx, store)
	c.Assert(err, gc.IsNil)

	info, err := store.ReadInfo("erewhemos")
	c.Assert(err, gc.IsNil)
	c.Assert(info, gc.NotNil)
	c.Assert(info.APIEndpoint().CACert, gc.Not(gc.Equals), "")
	c.Assert(info.APIEndpoint().EnvironUUID, gc.Not(gc.Equals), "")
	c.Assert(info.APICredentials().Password, gc.Not(gc.Equals), "")
	c.Assert(info.APICredentials().User, gc.Equals, "admin")
}

func (*OpenSuite) TestNewUnknownEnviron(c *gc.C) {
	attrs := dummySampleConfig().Merge(testing.Attrs{
		"type": "wondercloud",
	})
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, gc.ErrorMatches, "no registered provider for.*")
	c.Assert(env, gc.IsNil)
}

func (*OpenSuite) TestNewFromName(c *gc.C) {
	store := configstore.NewMem()
	ctx := testing.Context(c)
	e, err := environs.PrepareFromName("erewhemos", ctx, store)
	c.Assert(err, gc.IsNil)

	e, err = environs.NewFromName("erewhemos", store)
	c.Assert(err, gc.IsNil)
	c.Assert(e.Config().Name(), gc.Equals, "erewhemos")
}

func (*OpenSuite) TestNewFromNameWithInvalidInfo(c *gc.C) {
	store := configstore.NewMem()
	cfg, _, err := environs.ConfigForName("erewhemos", store)
	c.Assert(err, gc.IsNil)
	info := store.CreateInfo("erewhemos")

	// The configuration from environments.yaml is invalid
	// because it doesn't contain the state-id attribute which
	// the dummy environment adds at Prepare time.
	info.SetBootstrapConfig(cfg.AllAttrs())
	err = info.Write()
	c.Assert(err, gc.IsNil)

	e, err := environs.NewFromName("erewhemos", store)
	c.Assert(err, gc.ErrorMatches, "environment is not prepared")
	c.Assert(e, gc.IsNil)
}

func (*OpenSuite) TestNewFromNameWithInvalidEnvironConfig(c *gc.C) {
	store := configstore.NewMem()

	e, err := environs.NewFromName("erewhemos", store)
	c.Assert(err, gc.Equals, environs.ErrNotBootstrapped)
	c.Assert(e, gc.IsNil)
}

func (*OpenSuite) TestPrepareFromName(c *gc.C) {
	ctx := testing.Context(c)
	e, err := environs.PrepareFromName("erewhemos", ctx, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	c.Assert(e.Config().Name(), gc.Equals, "erewhemos")
	// Check we can access storage ok, which implies the environment has been prepared.
	c.Assert(e.Storage(), gc.NotNil)
}

func (*OpenSuite) TestConfigForName(c *gc.C) {
	cfg, source, err := environs.ConfigForName("erewhemos", configstore.NewMem())
	c.Assert(err, gc.IsNil)
	c.Assert(source, gc.Equals, environs.ConfigFromEnvirons)
	c.Assert(cfg.Name(), gc.Equals, "erewhemos")
}

func (*OpenSuite) TestConfigForNameNoDefault(c *gc.C) {
	cfg, source, err := environs.ConfigForName("", configstore.NewMem())
	c.Assert(err, gc.ErrorMatches, "no default environment found")
	c.Assert(cfg, gc.IsNil)
	c.Assert(source, gc.Equals, environs.ConfigFromEnvirons)
}

func (*OpenSuite) TestConfigForNameDefault(c *gc.C) {
	testing.WriteEnvironments(c, testing.SingleEnvConfig)
	cfg, source, err := environs.ConfigForName("", configstore.NewMem())
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.Name(), gc.Equals, "erewhemos")
	c.Assert(source, gc.Equals, environs.ConfigFromEnvirons)
}

func (*OpenSuite) TestConfigForNameFromInfo(c *gc.C) {
	testing.WriteEnvironments(c, testing.SingleEnvConfig)
	store := configstore.NewMem()
	cfg, source, err := environs.ConfigForName("", store)
	c.Assert(err, gc.IsNil)
	c.Assert(source, gc.Equals, environs.ConfigFromEnvirons)

	info := store.CreateInfo("test-config")
	var attrs testing.Attrs = cfg.AllAttrs()
	attrs = attrs.Merge(testing.Attrs{
		"name": "test-config",
	})
	info.SetBootstrapConfig(attrs)
	err = info.Write()
	c.Assert(err, gc.IsNil)

	cfg, source, err = environs.ConfigForName("test-config", store)
	c.Assert(err, gc.IsNil)
	c.Assert(source, gc.Equals, environs.ConfigFromInfo)
	c.Assert(testing.Attrs(cfg.AllAttrs()), gc.DeepEquals, attrs)
}

func (*OpenSuite) TestNew(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"state-server": false,
			"name":         "erewhemos",
		},
	))
	c.Assert(err, gc.IsNil)
	e, err := environs.New(cfg)
	c.Assert(err, gc.ErrorMatches, "environment is not prepared")
	c.Assert(e, gc.IsNil)
}

func (*OpenSuite) TestPrepare(c *gc.C) {
	baselineAttrs := dummy.SampleConfig().Merge(testing.Attrs{
		"state-server": false,
		"name":         "erewhemos",
	}).Delete(
		"ca-cert",
		"ca-private-key",
		"admin-secret",
		"uuid",
	)
	cfg, err := config.New(config.NoDefaults, baselineAttrs)
	c.Assert(err, gc.IsNil)
	store := configstore.NewMem()
	ctx := testing.Context(c)
	env, err := environs.Prepare(cfg, ctx, store)
	c.Assert(err, gc.IsNil)
	// Check we can access storage ok, which implies the environment has been prepared.
	c.Assert(env.Storage(), gc.NotNil)

	// Check that the environment info file was correctly created.
	info, err := store.ReadInfo("erewhemos")
	c.Assert(err, gc.IsNil)
	c.Assert(info.Initialized(), jc.IsTrue)
	c.Assert(info.BootstrapConfig(), gc.DeepEquals, env.Config().AllAttrs())
	c.Logf("bootstrap config: %#v", info.BootstrapConfig())

	// Check that an admin-secret was chosen.
	adminSecret := env.Config().AdminSecret()
	c.Assert(adminSecret, gc.HasLen, 32)
	c.Assert(adminSecret, gc.Matches, "^[0-9a-f]*$")

	// Check that the CA cert was generated.
	cfgCertPEM, cfgCertOK := env.Config().CACert()
	cfgKeyPEM, cfgKeyOK := env.Config().CAPrivateKey()
	c.Assert(cfgCertOK, gc.Equals, true)
	c.Assert(cfgKeyOK, gc.Equals, true)

	// Check the common name of the generated cert
	caCert, _, err := cert.ParseCertAndKey(cfgCertPEM, cfgKeyPEM)
	c.Assert(err, gc.IsNil)
	c.Assert(caCert.Subject.CommonName, gc.Equals, `juju-generated CA for environment "`+testing.SampleEnvName+`"`)

	// Check that a uuid was chosen.
	uuid, exists := env.Config().UUID()
	c.Assert(exists, gc.Equals, true)
	c.Assert(uuid, gc.Matches, `[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)

	// Check we can call Prepare again.
	env, err = environs.Prepare(cfg, ctx, store)
	c.Assert(err, gc.IsNil)
	c.Assert(env.Storage(), gc.NotNil)
	c.Assert(env.Config().AllAttrs(), gc.DeepEquals, info.BootstrapConfig())
}

func (*OpenSuite) TestPrepareGeneratesDifferentAdminSecrets(c *gc.C) {
	baselineAttrs := dummy.SampleConfig().Merge(testing.Attrs{
		"state-server": false,
		"name":         "erewhemos",
	}).Delete(
		"admin-secret",
	)
	cfg, err := config.New(config.NoDefaults, baselineAttrs)
	c.Assert(err, gc.IsNil)

	ctx := testing.Context(c)
	env0, err := environs.Prepare(cfg, ctx, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	adminSecret0 := env0.Config().AdminSecret()
	c.Assert(adminSecret0, gc.HasLen, 32)
	c.Assert(adminSecret0, gc.Matches, "^[0-9a-f]*$")

	env1, err := environs.Prepare(cfg, ctx, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	adminSecret1 := env1.Config().AdminSecret()
	c.Assert(adminSecret1, gc.HasLen, 32)
	c.Assert(adminSecret1, gc.Matches, "^[0-9a-f]*$")

	c.Assert(adminSecret1, gc.Not(gc.Equals), adminSecret0)
}

func (*OpenSuite) TestPrepareWithMissingKey(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Delete("ca-cert", "ca-private-key").Merge(
		testing.Attrs{
			"state-server": false,
			"name":         "erewhemos",
			"ca-cert":      string(testing.CACert),
		},
	))
	c.Assert(err, gc.IsNil)
	store := configstore.NewMem()
	env, err := environs.Prepare(cfg, testing.Context(c), store)
	c.Assert(err, gc.ErrorMatches, "cannot ensure CA certificate: environment configuration with a certificate but no CA private key")
	c.Assert(env, gc.IsNil)
	// Ensure that the config storage info is cleaned up.
	_, err = store.ReadInfo(cfg.Name())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (*OpenSuite) TestPrepareWithExistingKeyPair(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"state-server":   false,
			"name":           "erewhemos",
			"ca-cert":        string(testing.CACert),
			"ca-private-key": string(testing.CAKey),
		},
	))
	c.Assert(err, gc.IsNil)
	ctx := testing.Context(c)
	env, err := environs.Prepare(cfg, ctx, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	cfgCertPEM, cfgCertOK := env.Config().CACert()
	cfgKeyPEM, cfgKeyOK := env.Config().CAPrivateKey()
	c.Assert(cfgCertOK, gc.Equals, true)
	c.Assert(cfgKeyOK, gc.Equals, true)
	c.Assert(string(cfgCertPEM), gc.DeepEquals, testing.CACert)
	c.Assert(string(cfgKeyPEM), gc.DeepEquals, testing.CAKey)
}

func (*OpenSuite) TestDestroy(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"state-server": false,
			"name":         "erewhemos",
		},
	))
	c.Assert(err, gc.IsNil)

	store := configstore.NewMem()
	// Prepare the environment and sanity-check that
	// the config storage info has been made.
	ctx := testing.Context(c)
	e, err := environs.Prepare(cfg, ctx, store)
	c.Assert(err, gc.IsNil)
	_, err = store.ReadInfo(e.Config().Name())
	c.Assert(err, gc.IsNil)

	err = environs.Destroy(e, store)
	c.Assert(err, gc.IsNil)

	// Check that the environment has actually been destroyed
	// and that the config info has been destroyed too.
	_, err = e.StateServerInstances()
	c.Assert(err, gc.ErrorMatches, "environment has been destroyed")
	_, err = store.ReadInfo(e.Config().Name())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (*OpenSuite) TestNewFromAttrs(c *gc.C) {
	e, err := environs.NewFromAttrs(dummy.SampleConfig().Merge(
		testing.Attrs{
			"state-server": false,
			"name":         "erewhemos",
		},
	))
	c.Assert(err, gc.ErrorMatches, "environment is not prepared")
	c.Assert(e, gc.IsNil)
}

const checkEnv = `
environments:
    test:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`

type checkEnvironmentSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&checkEnvironmentSuite{})

func (s *checkEnvironmentSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	testing.WriteEnvironments(c, checkEnv)
}

func (s *checkEnvironmentSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.FakeJujuHomeSuite.TearDownTest(c)
}

func (s *checkEnvironmentSuite) TestCheckEnvironment(c *gc.C) {
	ctx := testing.Context(c)
	environ, err := environs.PrepareFromName("test", ctx, configstore.NewMem())
	c.Assert(err, gc.IsNil)

	// VerifyStorage is sufficient for our tests and much simpler
	// than Bootstrap which calls it.
	stor := environ.Storage()
	err = environs.VerifyStorage(stor)
	c.Assert(err, gc.IsNil)
	err = environs.CheckEnvironment(environ)
	c.Assert(err, gc.IsNil)
}

func (s *checkEnvironmentSuite) TestCheckEnvironmentFileNotFound(c *gc.C) {
	ctx := testing.Context(c)
	environ, err := environs.PrepareFromName("test", ctx, configstore.NewMem())
	c.Assert(err, gc.IsNil)

	// VerifyStorage is sufficient for our tests and much simpler
	// than Bootstrap which calls it.
	stor := environ.Storage()
	err = environs.VerifyStorage(stor)
	c.Assert(err, gc.IsNil)

	// When the bootstrap-verify file does not exist, it still believes
	// the environment is a juju-core one because earlier versions
	// did not create that file.
	err = stor.Remove(environs.VerificationFilename)
	c.Assert(err, gc.IsNil)
	err = environs.CheckEnvironment(environ)
	c.Assert(err, gc.IsNil)
}

func (s *checkEnvironmentSuite) TestCheckEnvironmentGetFails(c *gc.C) {
	ctx := testing.Context(c)
	environ, err := environs.PrepareFromName("test", ctx, configstore.NewMem())
	c.Assert(err, gc.IsNil)

	// VerifyStorage is sufficient for our tests and much simpler
	// than Bootstrap which calls it.
	stor := environ.Storage()
	err = environs.VerifyStorage(stor)
	c.Assert(err, gc.IsNil)

	// When fetching the verification file from storage fails,
	// we get an InvalidEnvironmentError.
	someError := errors.Unauthorizedf("you shall not pass")
	dummy.Poison(stor, environs.VerificationFilename, someError)
	err = environs.CheckEnvironment(environ)
	c.Assert(err, gc.Equals, someError)
}

func (s *checkEnvironmentSuite) TestCheckEnvironmentBadContent(c *gc.C) {
	ctx := testing.Context(c)
	environ, err := environs.PrepareFromName("test", ctx, configstore.NewMem())
	c.Assert(err, gc.IsNil)

	// We mock a bad (eg. from a Python-juju environment) bootstrap-verify.
	stor := environ.Storage()
	content := "bad verification content"
	reader := strings.NewReader(content)
	err = stor.Put(environs.VerificationFilename, reader, int64(len(content)))
	c.Assert(err, gc.IsNil)

	// When the bootstrap-verify file contains unexpected content,
	// we get an InvalidEnvironmentError.
	err = environs.CheckEnvironment(environ)
	c.Assert(err, gc.Equals, environs.InvalidEnvironmentError)
}
