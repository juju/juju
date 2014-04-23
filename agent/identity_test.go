// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"io/ioutil"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/version"
)

type identitySuite struct {
	testbase.LoggingSuite
	mongodConfigPath string
	mongodPath       string
}

var _ = gc.Suite(&identitySuite{})

var attributeParams = AgentConfigParams{
	Tag:               "omg",
	UpgradedToVersion: version.Current.Number,
	Password:          "sekrit",
	CACert:            "ca cert",
	StateAddresses:    []string{"localhost:1234"},
	APIAddresses:      []string{"localhost:1235"},
	Nonce:             "a nonce",
}

var servingInfo = params.StateServingInfo{
	Cert:           "old cert",
	PrivateKey:     "old key",
	StatePort:      69,
	APIPort:        47,
	SharedSecret:   "shared",
	SystemIdentity: "identity",
}

func (s *identitySuite) TestWriteSystemIdentityFile(c *gc.C) {
	params := attributeParams
	params.DataDir = c.MkDir()
	conf, err := NewStateMachineConfig(params, servingInfo)
	c.Assert(err, gc.IsNil)
	err = WriteSystemIdentityFile(conf)
	c.Assert(err, gc.IsNil)

	contents, err := ioutil.ReadFile(conf.SystemIdentityPath())
	c.Assert(err, gc.IsNil)
	c.Assert(string(contents), gc.Equals, servingInfo.SystemIdentity)
}
