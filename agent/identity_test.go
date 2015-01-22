// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"io/ioutil"
	"os"
	"runtime"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type identitySuite struct {
	testing.BaseSuite
	mongodConfigPath string
	mongodPath       string
}

var _ = gc.Suite(&identitySuite{})

var attributeParams = AgentConfigParams{
	Tag:               names.NewMachineTag("1"),
	UpgradedToVersion: version.Current.Number,
	Password:          "sekrit",
	CACert:            "ca cert",
	StateAddresses:    []string{"localhost:1234"},
	APIAddresses:      []string{"localhost:1235"},
	Nonce:             "a nonce",
	Environment:       testing.EnvironmentTag,
}

var servingInfo = params.StateServingInfo{
	Cert:           "old cert",
	PrivateKey:     "old key",
	CAPrivateKey:   "old ca key",
	StatePort:      69,
	APIPort:        47,
	SharedSecret:   "shared",
	SystemIdentity: "identity",
}

func (s *identitySuite) TestWriteSystemIdentityFile(c *gc.C) {
	params := attributeParams
	params.DataDir = c.MkDir()
	conf, err := NewStateMachineConfig(params, servingInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = WriteSystemIdentityFile(conf)
	c.Assert(err, jc.ErrorIsNil)

	contents, err := ioutil.ReadFile(conf.SystemIdentityPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(contents), gc.Equals, servingInfo.SystemIdentity)

	fi, err := os.Stat(conf.SystemIdentityPath())
	c.Assert(err, jc.ErrorIsNil)

	// Windows is not fully POSIX compliant. Chmod() and Chown() have unexpected behavior
	// compared to linux/unix
	if runtime.GOOS != "windows" {
		c.Check(fi.Mode().Perm(), gc.Equals, os.FileMode(0600))
	}
	// ensure that file is deleted when SystemIdentity is empty
	info := servingInfo
	info.SystemIdentity = ""
	conf, err = NewStateMachineConfig(params, info)
	c.Assert(err, jc.ErrorIsNil)
	err = WriteSystemIdentityFile(conf)
	c.Assert(err, jc.ErrorIsNil)
	fi, err = os.Stat(conf.SystemIdentityPath())
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}
