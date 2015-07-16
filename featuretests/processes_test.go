// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/component/all"
	"github.com/juju/juju/process"
	"github.com/juju/juju/testcharms"
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
	testing.IsolationSuite

	env *procsEnviron
}

func (s *processesBaseSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)

	s.env = newProcsEnv(c, "local")
	s.env.bootstrap(c)
}

func (s *processesBaseSuite) TearDownSuite(c *gc.C) {
	s.env.destroy(c)

	s.IsolationSuite.TearDownSuite(c)
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

	unit.setConfigStatus(c, "okay")

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
	name    string
	machine string
	exec    func(cmd string, args ...string) (string, error)
}

func newProcsEnv(c *gc.C, envName string) *procsEnviron {
	return &procsEnviron{
		name:    envName,
		machine: "1",
		exec: func(cmd string, args ...string) (string, error) {
			c.Logf("running %s %s", cmd, strings.Join(args, " "))
			return "", nil
			command := exec.Command(cmd, args...)
			out, err := command.CombinedOutput()
			return string(out), err
		},
	}
}

func (env *procsEnviron) run(c *gc.C, cmd string, args ...string) string {
	args = append(append(strings.Fields(cmd), "--environment="+env.name), args...)

	// TODO(ericsnow) This isn't ideal...
	out, err := env.exec("juju", args...)
	c.Assert(err, jc.ErrorIsNil)
	return out
}

func (env *procsEnviron) bootstrap(c *gc.C) {
	env.run(c, "bootstrap")
}

func (env *procsEnviron) addService(c *gc.C, charmName, serviceName string) *procsService {
	if serviceName == "" {
		serviceName = charmName
	}
	charmURL := "local:quantal/" + charmName
	repoDir := testcharms.Repo.Path()

	env.run(c, "add-machine")
	env.run(c, "deploy", "--to="+env.machine, "--repository="+repoDir, charmURL, serviceName)
	env.run(c, "destroy-unit", serviceName+"/0")

	return &procsService{
		env:       env,
		charmName: charmName,
		name:      serviceName,
	}
}

func (env *procsEnviron) destroy(c *gc.C) {
	env.run(c, "destroy-environment", "-f")
}

type procsService struct {
	env       *procsEnviron
	charmName string
	name      string
	lastUnit  int
}

func (svc *procsService) setConfig(c *gc.C, settings map[string]string) {
	var args []string
	for k, v := range settings {
		args = append(args, fmt.Sprintf("%s=%q", k, v))
	}
	svc.env.run(c, "service set", args...)
}

func (svc *procsService) deploy(c *gc.C, procName, pluginID, status string) *procsUnit {
	settings := map[string]string{
		"plugin-name":   procName,
		"plugin-id":     pluginID,
		"plugin-status": status,
	}
	svc.setConfig(c, settings)

	svc.env.run(c, "service add-unit", "--to="+svc.env.machine, svc.name)

	// TODO(ericsnow) wait until ready...

	svc.lastUnit += 1
	return &procsUnit{
		svc: svc,
		id:  fmt.Sprintf("%s/%d", svc.name, svc.lastUnit),
	}
}

type procsUnit struct {
	svc *procsService
	id  string
}

func (u *procsUnit) setConfig(c *gc.C, settings map[string]string) {
	u.svc.setConfig(c, settings)
}

func (u *procsUnit) setConfigStatus(c *gc.C, status string) {
	settings := map[string]string{"plugin-status": status}
	u.setConfig(c, settings)
}

func (u *procsUnit) destroy(c *gc.C) {
	u.svc.env.run(c, "destroy-unit", u.id)

	// TODO(ericsnow) wait until ready...
}

func (u *procsUnit) runAction(c *gc.C, action string, actionArgs map[string]interface{}) map[string]string {
	// Send the command.
	args := []string{
		u.id,
		action,
	}
	for k, v := range actionArgs {
		args = append(args, fmt.Sprintf("%s=%q", k, v))
	}
	doOut := u.svc.env.run(c, "action do", args...)
	c.Assert(strings.Fields(doOut), gc.HasLen, 2)
	actionID := strings.Fields(doOut)[1]

	// Get the results.
	fetchOut := u.svc.env.run(c, "action fetch", "--wait", actionID)
	result := struct {
		Result map[string]interface{}
	}{}
	err := goyaml.Unmarshal([]byte(fetchOut), &result)
	c.Assert(err, jc.ErrorIsNil)
	rawResults := result.Result

	// Check and coerce the results.
	c.Check(rawResults["status"].(string), gc.Equals, "success")
	delete(rawResults, "status")
	results := make(map[string]string, len(rawResults))
	for k, v := range rawResults {
		results[k] = v.(string)
	}
	return results
}

func (u *procsUnit) injectStatus(c *gc.C, pluginID, status string) {
	args := map[string]interface{}{
		"id":     pluginID,
		"status": status,
	}
	u.runAction(c, "plugin-setstatus", args)
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
	var procs []process.Info
	// TODO(ericsnow) finish!
	c.Check(procs, jc.DeepEquals, expected)
}

func (u *procsUnit) checkPluginLog(c *gc.C, expected []string) {
	results := u.runAction(c, "plugin-dump", nil)
	c.Assert(results, gc.HasLen, 1)
	output, ok := results["output"]
	c.Assert(ok, jc.IsTrue)
	lines := strings.Split(output, "\n")
	// TODO(ericsnow) strip header...

	c.Check(lines, jc.DeepEquals, expected)
}
