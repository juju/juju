// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"os"
	"path"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/goleak"

	"github.com/juju/juju/internal/testhelpers"
)

type configSuite struct {
	testhelpers.IsolationSuite

	configPath string
}

func TestConfigSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	tc.Run(t, &configSuite{})
}

func (s *configSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.configPath = path.Join(c.MkDir(), "controller.conf")
}

func (s *configSuite) TestReadConfigSuccess(c *tc.C) {
	data := `
db-bind-addresses:
  controller/0: 10.246.27.225
  controller/1: 10.246.27.167
  controller/2: 10.246.27.218`[1:]

	err := os.WriteFile(s.configPath, []byte(data), 0644)
	c.Assert(err, tc.ErrorIsNil)

	addrs, err := controllerConfigReader{configPath: s.configPath}.DBBindAddresses()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(addrs, tc.DeepEquals, map[string]string{
		"controller/0": "10.246.27.225",
		"controller/1": "10.246.27.167",
		"controller/2": "10.246.27.218",
	})
}

func (s *configSuite) TestReadConfigWrongFileError(c *tc.C) {
	addrs, err := controllerConfigReader{configPath: s.configPath}.DBBindAddresses()
	c.Check(addrs, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "reading config file from .*")
}

func (s *configSuite) TestReadConfigBadContentsError(c *tc.C) {
	err := os.WriteFile(s.configPath, []byte("can't parse this do-do-do-do"), 0644)
	c.Assert(err, tc.ErrorIsNil)

	addrs, err := controllerConfigReader{configPath: s.configPath}.DBBindAddresses()
	c.Check(addrs, tc.IsNil)
	c.Log(err.Error())
	c.Check(err, tc.ErrorMatches, "parsing config file (.|\\n)*")
}
