// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/component/all"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/process"
	coretesting "github.com/juju/juju/testing"
)

func initProcessesSuites() {
	if err := all.RegisterForServer(); err != nil {
		panic(err)
	}

	gc.Suite(&processesHookContextSuite{})
	gc.Suite(&processesWorkerSuite{})
	gc.Suite(&processesCmdJujuSuite{})
}

type processesBaseSuite struct {
	jujutesting.JujuConnSuite

	env  *procsEnviron
	unit *procsUnit
}

func (s *processesBaseSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)

	s.env = newProcsEnv(c, s, "") // TODO(ericsnow) switch to "local"...
}

func (s *processesBaseSuite) TearDownSuite(c *gc.C) {
	s.env.destroy(c)

	s.JujuConnSuite.TearDownSuite(c)
}

type processesHookContextSuite struct {
	processesBaseSuite
}

func (s *processesHookContextSuite) TestHookLifecycle(c *gc.C) {
	// start: info, launch, info
	// config-changed: info, set-status, info
	// stop: info, destroy, info

	svc := s.env.addService(c, "proc-hooks", "a-service")

	// Add/start the unit.

	unit := svc.deploy(c, "myproc", "xyz123", "running")

	unit.checkState(c, []process.Info{{
		Process: charm.Process{
			Name: "myproc",
			Type: "myplugin",
		},
		Details: process.Details{
			ID: "xyz123",
			Status: process.PluginStatus{
				Label: "running",
			},
		},
	}})
	unit.checkPluginLog(c, []string{
		"...",
	})

	// Change the config.

	unit.setStatus(c, "okay")

	unit.checkState(c, []process.Info{{
		Process: charm.Process{
			Name: "myproc",
			Type: "myplugin",
		},
		Details: process.Details{
			ID: "xyz123",
			Status: process.PluginStatus{
				Label: "okay",
			},
		},
	}})
	unit.checkPluginLog(c, []string{
		"...",
	})

	// Stop the unit.

	unit.destroy(c)

	unit.checkState(c, nil)
	unit.checkPluginLog(c, []string{
		"...",
	})
}

// TODO(ericsnow) Add a test specifically for each supported plugin
// (e.g. docker)?

func (s *processesHookContextSuite) TestRegister(c *gc.C) {
	//s.addUnit(c, "proc-actions", "a-service")
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesHookContextSuite) TestLaunch(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesHookContextSuite) TestInfo(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesHookContextSuite) TestUnregister(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesHookContextSuite) TestDestroy(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

type processesWorkerSuite struct {
	processesBaseSuite
}

func (s *processesWorkerSuite) TestSetStatus(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesWorkerSuite) TestCleanUp(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

type processesCmdJujuSuite struct {
	processesBaseSuite
}

func (s *processesCmdJujuSuite) TestStatus(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

type procsEnviron struct {
	base coretesting.Environ
}

func newProcsEnv(c *gc.C, s *processesBaseSuite, envName string) *procsEnviron {
	base := jujutesting.NewEnv(&s.JujuConnSuite)
	if envName != "" {
		base = coretesting.NewEnv(c, envName)
	}
	return &procsEnviron{base: base}
}

func (env *procsEnviron) addService(c *gc.C, charmName, serviceName string) *procsService {
	svc := env.base.AddService(c, charmName, serviceName)
	return &procsService{base: svc}
}

func (env *procsEnviron) destroy(c *gc.C) {
	env.base.Destroy(c)
}

type procsService struct {
	base coretesting.Service
}

func (svc *procsService) setConfig(c *gc.C, settings map[string]string) {
	svc.base.SetConfig(c, settings)
}

func (svc *procsService) deploy(c *gc.C, procName, pluginID, status string) *procsUnit {
	settings := map[string]string{
		"plugin-name":   procName,
		"plugin-id":     pluginID,
		"plugin-status": status,
	}
	svc.setConfig(c, settings)

	unit := svc.base.Deploy(c)
	return &procsUnit{base: unit, svc: svc}
}

type procsUnit struct {
	base coretesting.Unit
	svc  *procsService
}

func (u *procsUnit) setConfig(c *gc.C, settings map[string]string) {
	u.base.SetConfig(c, settings)
}

func (u *procsUnit) runAction(c *gc.C, name string, args map[string]interface{}) map[string]string {
	rawResults := u.base.RunAction(c, name, args)
	delete(rawResults, "status")
	results := make(map[string]string, len(rawResults))
	for k, v := range rawResults {
		results[k] = v.(string)
	}
	return results
}

func (u *procsUnit) destroy(c *gc.C) {
	u.base.Destroy(c)
}

func (u *procsUnit) setStatus(c *gc.C, status string) {
	settings := map[string]string{"plugin-status": status}
	u.setConfig(c, settings)
}

func (u *procsUnit) prepPlugin(c *gc.C, procName, pluginID, status string) {
	args := map[string]interface{}{
		"name":   procName,
		"id":     pluginID,
		"status": status,
	}
	u.runAction(c, "plugin-prep", args)
}

func (u *procsUnit) checkState(c *gc.C, expected []process.Info) {
	procs := u.base.Procs(c)
	c.Check(procs, jc.DeepEquals, expected)
}

func (u *procsUnit) checkPluginLog(c *gc.C, expected []string) {
	results := u.runAction(c, "plugin-dump", nil)
	c.Assert(results, gc.HasLen, 1)
	output, ok := results["output"]
	c.Assert(ok, jc.IsTrue)
	lines := strings.Split(output, "\n")

	c.Check(lines, jc.DeepEquals, expected)
}
