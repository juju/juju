// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/reboot"
	"github.com/juju/juju/core/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	svctesting "github.com/juju/juju/service/common/testing"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type RebootSuite struct {
	jujutesting.JujuConnSuite

	acfg    agent.Config
	mgoInst testing.MgoInstance
	st      api.Connection

	tmpDir           string
	rebootScriptName string

	services    []*svctesting.FakeService
	serviceData *svctesting.FakeServiceData
}

var _ = gc.Suite(&RebootSuite{})

func (s *RebootSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)

	// These tests only patch out LXC, so only run full-stack tests
	// over LXC.
	s.PatchValue(&instance.ContainerTypes, []instance.ContainerType{instance.LXD})
}

func (s *RebootSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	testing.PatchExecutableAsEchoArgs(c, s, rebootBin)
	s.PatchEnvironment("TEMP", c.MkDir())

	s.tmpDir = c.MkDir()
	s.rebootScriptName = "juju-reboot-script"
	s.PatchValue(reboot.TmpFile, func() (*os.File, error) {
		script := s.rebootScript(c)
		return os.Create(script)
	})

	s.mgoInst.EnableAuth = true
	err := s.mgoInst.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)

	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: c.MkDir()},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: jujuversion.Current,
		APIAddresses:      []string{"localhost:17070"},
		CACert:            coretesting.CACert,
		Password:          "fake",
		Controller:        s.State.ControllerTag(),
		Model:             s.Model.ModelTag(),
		MongoVersion:      mongo.Mongo24,
	}
	s.st, _ = s.OpenAPIAsNewMachine(c)

	s.acfg, err = agent.NewAgentConfig(configParams)
	c.Assert(err, jc.ErrorIsNil)
	fakeServices := []string{
		"jujud-machine-1",
		"jujud-unit-drupal-1",
		"jujud-unit-mysql-1",
		"fake-random-service",
	}
	for _, fake := range fakeServices {
		s.addService(fake)
	}
	testing.PatchValue(&service.NewService, s.newService)
	testing.PatchValue(&service.ListServices, s.listServices)
}

func (s *RebootSuite) addService(name string) {
	svc, _ := s.newService(name, common.Conf{}, "")
	svc.Install()
	svc.Start()
}

func (s *RebootSuite) listServices() ([]string, error) {
	return s.serviceData.InstalledNames(), nil
}

func (s *RebootSuite) newService(name string, conf common.Conf, series string) (service.Service, error) {
	for _, svc := range s.services {
		if svc.Name() == name {
			return svc, nil
		}
	}
	if s.serviceData == nil {
		s.serviceData = svctesting.NewFakeServiceData()
	}
	svc := &svctesting.FakeService{
		FakeServiceData: s.serviceData,
		Service: common.Service{
			Name: name,
			Conf: common.Conf{},
		},
	}
	s.services = append(s.services, svc)
	return svc, nil
}

func (s *RebootSuite) TestRebootStopUnits(c *gc.C) {
	w, err := reboot.NewRebootWaiter(s.acfg)
	c.Assert(err, jc.ErrorIsNil)

	err = w.ExecuteReboot(params.ShouldReboot)
	c.Assert(err, jc.ErrorIsNil)

	for _, svc := range s.services {
		name := svc.Name()
		if strings.HasPrefix(name, `jujud-unit-`) {
			running, err := svc.Running()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(running, jc.IsFalse)
		} else {
			running, err := svc.Running()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(running, jc.IsTrue)
		}
	}
}

func (s *RebootSuite) TearDownTest(c *gc.C) {
	s.mgoInst.Destroy()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *RebootSuite) rebootScript(c *gc.C) string {
	return filepath.Join(s.tmpDir, s.rebootScriptName)
}
