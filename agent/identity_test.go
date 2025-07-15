// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/testing"
)

type identitySuite struct {
	testing.BaseSuite
}

func TestIdentitySuite(t *stdtesting.T) {
	tc.Run(t, &identitySuite{})
}

var attributeParams = AgentConfigParams{
	Tag:               names.NewMachineTag("1"),
	UpgradedToVersion: jujuversion.Current,
	Password:          "sekrit",
	CACert:            "ca cert",
	APIAddresses:      []string{"localhost:1235"},
	Nonce:             "a nonce",
	Controller:        testing.ControllerTag,
	Model:             testing.ModelTag,
}

var servingInfo = controller.StateServingInfo{
	Cert:           "old cert",
	PrivateKey:     "old key",
	CAPrivateKey:   "old ca key",
	APIPort:        47,
	SystemIdentity: "identity",
}

func (s *identitySuite) TestWriteSystemIdentityFile(c *tc.C) {
	params := attributeParams
	params.Paths.DataDir = c.MkDir()
	conf, err := NewStateMachineConfig(params, servingInfo)
	c.Assert(err, tc.ErrorIsNil)
	err = WriteSystemIdentityFile(conf)
	c.Assert(err, tc.ErrorIsNil)

	contents, err := os.ReadFile(conf.SystemIdentityPath())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Equals, servingInfo.SystemIdentity)

	fi, err := os.Stat(conf.SystemIdentityPath())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fi.Mode().Perm(), tc.Equals, os.FileMode(0600))
	// ensure that file is deleted when SystemIdentity is empty
	info := servingInfo
	info.SystemIdentity = ""
	conf, err = NewStateMachineConfig(params, info)
	c.Assert(err, tc.ErrorIsNil)
	err = WriteSystemIdentityFile(conf)
	c.Assert(err, tc.ErrorIsNil)
	_, err = os.Stat(conf.SystemIdentityPath())
	c.Assert(err, tc.Satisfies, os.IsNotExist)
}
