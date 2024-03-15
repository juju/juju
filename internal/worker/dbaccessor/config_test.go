package dbaccessor

import (
	"os"
	"path"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type configSuite struct {
	jujutesting.IsolationSuite

	configPath string
}

var _ = gc.Suite(&configSuite{})

func (s *configSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.configPath = path.Join(c.MkDir(), "controller.conf")
}

func (s *configSuite) TestReadConfigSuccess(c *gc.C) {
	data := `
db-bind-addresses:
  controller/0: 10.246.27.225
  controller/1: 10.246.27.167
  controller/2: 10.246.27.218`[1:]

	err := os.WriteFile(s.configPath, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)

	addrs, err := controllerConfigReader{configPath: s.configPath}.DBBindAddresses()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(addrs, gc.DeepEquals, map[string]string{
		"controller/0": "10.246.27.225",
		"controller/1": "10.246.27.167",
		"controller/2": "10.246.27.218",
	})
}

func (s *configSuite) TestReadConfigWrongFileError(c *gc.C) {
	addrs, err := controllerConfigReader{configPath: s.configPath}.DBBindAddresses()
	c.Check(addrs, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "reading config file from .*")
}

func (s *configSuite) TestReadConfigBadContentsError(c *gc.C) {
	err := os.WriteFile(s.configPath, []byte("can't parse this do-do-do-do"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	addrs, err := controllerConfigReader{configPath: s.configPath}.DBBindAddresses()
	c.Check(addrs, gc.IsNil)
	c.Log(err.Error())
	c.Check(err, gc.ErrorMatches, "parsing config file (.|\\n)*")
}
