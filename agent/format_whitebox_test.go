// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/version"
)

type formatSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&formatSuite{})

// The agentParams are used by the specific formatter whitebox tests, and is
// located here for easy reuse.
var agentParams = AgentConfigParams{
	Tag:               "omg",
	UpgradedToVersion: version.Current.Number,
	Jobs:              []params.MachineJob{params.JobHostUnits},
	Password:          "sekrit",
	CACert:            []byte("ca cert"),
	StateAddresses:    []string{"localhost:1234"},
	APIAddresses:      []string{"localhost:1235"},
	Nonce:             "a nonce",
}

func newTestConfig(c *gc.C) *configInternal {
	params := agentParams
	params.DataDir = c.MkDir()
	params.LogDir = c.MkDir()
	config, err := NewAgentConfig(params)
	c.Assert(err, gc.IsNil)
	return config.(*configInternal)
}

const legacyFormatFileContents = "format 1.16"
const legacyConfig1_16 = `
tag: omg
nonce: a nonce
cacert: Y2EgY2VydA==
stateaddresses:
- localhost:1234
apiaddresses:
- localhost:1235
oldpassword: sekrit
values: {}
`

func (*formatSuite) TestReadPreviousFormatWritesNew(c *gc.C) {
	config := newTestConfig(c)

	err := previousFormatter.write(config)
	c.Assert(err, gc.IsNil)

	_, err = ReadConf(ConfigPath(config.DataDir(), config.Tag()))
	c.Assert(err, gc.IsNil)
	format, err := readFormat(config.Dir())
	c.Assert(err, gc.IsNil)
	c.Assert(format, gc.Equals, currentFormat)
}
