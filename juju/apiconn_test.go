// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
)

type NewAPIConnSuite struct {
	coretesting.LoggingSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&NewAPIConnSuite{})

func (cs *NewAPIConnSuite) SetUpTest(c *gc.C) {
	cs.LoggingSuite.SetUpTest(c)
	cs.ToolsFixture.SetUpTest(c)
}

func (cs *NewAPIConnSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	cs.ToolsFixture.TearDownTest(c)
	cs.LoggingSuite.TearDownTest(c)
}

func (*NewAPIConnSuite) TestNewConn(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)

	conn, err := juju.NewAPIConn(env, api.DefaultDialOpts())
	c.Assert(err, gc.IsNil)

	c.Assert(conn.Environ, gc.Equals, env)
	c.Assert(conn.State, gc.NotNil)

	c.Assert(conn.Close(), gc.IsNil)
}

type NewAPIClientSuite struct {
	coretesting.LoggingSuite
}

var _ = gc.Suite(&NewAPIClientSuite{})

func (cs *NewAPIClientSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	cs.LoggingSuite.TearDownTest(c)
}

func (*NewAPIClientSuite) TestNameDefault(c *gc.C) {
	defer coretesting.MakeMultipleEnvHome(c).Restore()
	// The default environment is "erewhemos", we should get it if we ask for ""
	defaultEnvName := "erewhemos"
	bootstrapEnv(c, defaultEnvName)
	apiclient, err := juju.NewAPIClientFromName("")
	c.Assert(err, gc.IsNil)
	defer apiclient.Close()
	envInfo, err := apiclient.EnvironmentInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(envInfo.Name, gc.Equals, defaultEnvName)
}

func (*NewAPIClientSuite) TestNameNotDefault(c *gc.C) {
	defer coretesting.MakeMultipleEnvHome(c).Restore()
	// The default environment is "erewhemos", make sure we get the other one.
	const envName = "erewhemos-2"
	bootstrapEnv(c, envName)
	apiclient, err := juju.NewAPIClientFromName(envName)
	c.Assert(err, gc.IsNil)
	defer apiclient.Close()
	envInfo, err := apiclient.EnvironmentInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(envInfo.Name, gc.Equals, envName)
}

func (*NewAPIClientSuite) TestWithInfoOnly(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	creds := environs.APICredentials{
		User: "foo",
		Password: "foopass",
	}
	endpoint := environs.APIEndpoint{
		Addresses: []string{"foo.com"},
		CACert: "certificated",
	}
	defer setDefaultConfigStore("noconfig", &environInfo{
		creds: creds,
		endpoint: endpoint,
	}).Restore()

	called := 0
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (*api.State, error) {
		c.Check(apiInfo.Tag, gc.Equals, "user-foo")
		c.Check(string(apiInfo.CACert), gc.Equals, "certificated")
		c.Check(apiInfo.Tag, gc.Equals, "user-foo")
		c.Check(apiInfo.Password, gc.Equals, "foopass")
		c.Check(opts, gc.DeepEquals, api.DefaultDialOpts())
		called++
		return new(api.State), nil
	}
	defer jc.Set(juju.APIOpen, apiOpen).Restore()
	client, err := juju.NewAPIClientFromName("noconfig")
	c.Assert(err, gc.IsNil)
	c.Assert(client, gc.NotNil)
	c.Assert(called, gc.Equals, 1)
}

func (*NewAPIClientSuite) TestWithInfoError(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	expectErr := fmt.Errorf("an error")
	defer setDefaultConfigStore("noconfig", &environInfo{
		err: expectErr,
	}).Restore()
	defer jc.Set(juju.APIOpen, nil).Restore()
	client, err := juju.NewAPIClientFromName("noconfig")
	c.Assert(err, gc.Equals, expectErr)
	c.Assert(client, gc.IsNil)
}

func (*NewAPIClientSuite) TestWithInfoNoAddresses(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	endpoint := environs.APIEndpoint{
		Addresses: []string{},
		CACert: "certificated",
	}
	defer setDefaultConfigStore("noconfig", &environInfo{
		endpoint: endpoint,
	}).Restore()
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (*api.State, error) {
		panic("api.Open called unexpectedly")
	}
	defer jc.Set(juju.APIOpen, apiOpen).Restore()

	client, err := juju.NewAPIClientFromName("noconfig")
	c.Assert(err, gc.ErrorMatches, `environment "noconfig" not found`)
	c.Assert(client, gc.IsNil)
}

func (*NewAPIClientSuite) TestWithInfoAPIOpenError(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	endpoint := environs.APIEndpoint{
		Addresses: []string{"foo.com"},
	}
	defer setDefaultConfigStore("noconfig", &environInfo{
		endpoint: endpoint,
	}).Restore()

	expectErr := fmt.Errorf("an error")
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (*api.State, error) {
		return nil, expectErr
	}
	defer jc.Set(juju.APIOpen, apiOpen).Restore()
	client, err := juju.NewAPIClientFromName("noconfig")
	c.Assert(err, gc.Equals, expectErr)
	c.Assert(client, gc.IsNil)
}


//func (*NewAPIClientSuite) TestBothSlowInfo(c *gc.C) {
//}
//	set delay=small
//	should connect to info, then connect to config
//
//func (*NewAPIClientSuite) TestBothFastInfo(c *gc.C) {
//}
//	set delay=large
//	should connect to info and not connect to config.
//
//func (*NewAPIClientSuite) TestBothSlow(c *gc.C) {
//}
//	set delay=small
//	both should try to connect
//	let info connect
//	get result
//	let config connect
//
//func (*NewAPIClientSuite) TestBothErrror(c *gc.C) {
//}
//	should get error from config connect

// TODO(jam): 2013-08-27 This should move somewhere in api.*
func (*NewAPIClientSuite) TestMultipleCloseOk(c *gc.C) {
	defer coretesting.MakeSampleHome(c).Restore()
	bootstrapEnv(c, "")
	client, _ := juju.NewAPIClientFromName("")
	c.Assert(client.Close(), gc.IsNil)
	c.Assert(client.Close(), gc.IsNil)
	c.Assert(client.Close(), gc.IsNil)
}

func setDefaultConfigStore(envName string, info *environInfo) jc.Restorer {
	return jc.Set(juju.DefaultConfigStore, func() (environs.ConfigStorage, error) {
		return &configStorage{
			envs: map[string]*environInfo{
				envName: info,
			},
		}, nil
	})
}

type environInfo struct {
	creds environs.APICredentials
	endpoint environs.APIEndpoint
	err error
}

type configStorage struct {
	envs map[string] *environInfo
}

func (store *configStorage) ReadInfo(envName string) (environs.EnvironInfo, error) {
	info := store.envs[envName]
	if info == nil {
		return nil, errors.NotFoundf("info on environment %q", envName)
	}
	if info.err != nil {
		return nil, info.err
	}
	return info, nil
}

func (info *environInfo) APICredentials() environs.APICredentials {
	return info.creds
}

func (info *environInfo) APIEndpoint() environs.APIEndpoint {
	return info.endpoint
}
