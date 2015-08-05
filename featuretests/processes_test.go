// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/cmd/juju/service"
	"github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/component/all"
	jjj "github.com/juju/juju/juju"
	"github.com/juju/juju/process"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

func initProcessesSuites() {
	if err := all.RegisterForServer(); err != nil {
		panic(err)
	}

	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	// TODO(ericsnow) Why is it necessary to check for root?
	if u.Username == "root" {
		return
	}
	userInfo = u
	gc.Suite(&processesSuite{})
}

var (
	repoDir  = testcharms.Repo.Path()
	userInfo *user.User
)

type processesSuite struct {
	env *procsEnviron
	svc *procsService
}

func (s *processesSuite) SetUpSuite(c *gc.C) {
	env := newProcsEnv(c, "", "<local>")
	env.bootstrap(c)
	s.env = env
}

func (s *processesSuite) TearDownSuite(c *gc.C) {
	if s.env != nil {
		s.env.destroy(c)
	}
}

func (s *processesSuite) SetUpTest(c *gc.C) {
}

func (s *processesSuite) TearDownTest(c *gc.C) {
	if c.Failed() {
		s.env.markFailure(c, s)
	} else {
		if s.svc != nil {
			s.svc.destroy(c)
		}
	}
}

func (s *processesSuite) TestHookLifecycle(c *gc.C) {
	// start: info, launch, info
	// config-changed: info, set-status, info
	// stop: info, destroy, info

	svc := s.env.addService(c, "proc-hooks", "a-service")
	s.svc = svc

	// Add/start the unit.

	unit := svc.deploy(c, "myproc", "xyz123", "running")

	unit.checkState(c, []process.Info{{
		Process: charm.Process{
			Name: "myproc",
			Type: "myplugin",
			TypeOptions: map[string]string{
				"critical": "true",
			},
			Command: "run-server",
			Image:   "web-server",
			Ports: []charm.ProcessPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "",
			}, {
				External: 8081,
				Internal: 443,
				Endpoint: "",
			}},
			Volumes: []charm.ProcessVolume{{
				ExternalMount: "/var/some-server/html",
				InternalMount: "/usr/share/some-server/html",
				Mode:          "ro",
				Name:          "",
			}, {
				ExternalMount: "/var/some-server/conf",
				InternalMount: "/etc/some-server",
				Mode:          "ro",
				Name:          "",
			}},
			EnvVars: map[string]string{
				"IMPORTANT": "some value",
			},
		},
		Status: process.Status{
			State:   process.StateRunning,
			Blocker: "",
			Message: "",
		},
		Details: process.Details{
			ID: "xyz123",
			Status: process.PluginStatus{
				State: "running",
			},
		},
	}})
	unit.checkPluginLog(c, []string{
		`myproc	xyz123	running		added`,
		`myproc	xyz123	running	{"Name":"myproc","Description":"","Type":"myplugin","TypeOptions":{"critical":"true"},"Command":"run-server","Image":"web-server","Ports":[{"External":8080,"Internal":80,"Endpoint":""},{"External":8081,"Internal":443,"Endpoint":""}],"Volumes":[{"ExternalMount":"/var/some-server/html","InternalMount":"/usr/share/some-server/html","Mode":"ro","Name":""},{"ExternalMount":"/var/some-server/conf","InternalMount":"/etc/some-server","Mode":"ro","Name":""}],"EnvVars":{"IMPORTANT":"some value"}}	definition set`,
	})

	// Change the config.

	// TODO(ericsnow) Implement once proc-set-status exists...

	// Stop the unit.

	if c.Failed() {
		return
	}
	unit.destroy(c)

	// At this point we can no longer call unit.checkStatus
	// or unit.checkPluginLog...
}

// TODO(ericsnow) Add a test specifically for each supported plugin
// (e.g. docker)?

func (s *processesSuite) TestHookContextRegister(c *gc.C) {
	svc := s.env.addService(c, "proc-actions", "register-service")
	s.svc = svc

	args := map[string]interface{}{
		"name":   "myproc",
		"id":     "xyz123",
		"status": "running",
	}
	svc.dummy.runAction(c, "register", args)

	svc.dummy.checkState(c, []process.Info{{
		Process: charm.Process{
			Name: "myproc",
			Type: "myplugin",
			TypeOptions: map[string]string{
				"critical": "true",
			},
			Command: "run-server",
			Image:   "web-server",
			Ports: []charm.ProcessPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "",
			}, {
				External: 8081,
				Internal: 443,
				Endpoint: "",
			}},
			Volumes: []charm.ProcessVolume{{
				ExternalMount: "/var/some-server/html",
				InternalMount: "/usr/share/some-server/html",
				Mode:          "ro",
				Name:          "",
			}, {
				ExternalMount: "/var/some-server/conf",
				InternalMount: "/etc/some-server",
				Mode:          "ro",
				Name:          "",
			}},
			EnvVars: map[string]string{
				"IMPORTANT": "some value",
			},
		},
		Status: process.Status{
			State:   process.StateRunning,
			Blocker: "",
			Message: "",
		},
		Details: process.Details{
			ID: "xyz123",
			Status: process.PluginStatus{
				State: "running",
			},
		},
	}})
}

func (s *processesSuite) TestHookContextLaunch(c *gc.C) {
	svc := s.env.addService(c, "proc-actions", "launch-service")
	s.svc = svc

	svc.dummy.prepPlugin(c, "myproc", "xyz123", "running")

	args := map[string]interface{}{
		"name": "myproc",
	}
	svc.dummy.runAction(c, "launch", args)

	svc.dummy.checkState(c, []process.Info{{
		Process: charm.Process{
			Name: "myproc",
			Type: "myplugin",
			TypeOptions: map[string]string{
				"critical": "true",
			},
			Command: "run-server",
			Image:   "web-server",
			Ports: []charm.ProcessPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "",
			}, {
				External: 8081,
				Internal: 443,
				Endpoint: "",
			}},
			Volumes: []charm.ProcessVolume{{
				ExternalMount: "/var/some-server/html",
				InternalMount: "/usr/share/some-server/html",
				Mode:          "ro",
				Name:          "",
			}, {
				ExternalMount: "/var/some-server/conf",
				InternalMount: "/etc/some-server",
				Mode:          "ro",
				Name:          "",
			}},
			EnvVars: map[string]string{
				"IMPORTANT": "some value",
			},
		},
		Status: process.Status{
			State:   process.StateRunning,
			Blocker: "",
			Message: "",
		},
		Details: process.Details{
			ID: "xyz123",
			Status: process.PluginStatus{
				State: "running",
			},
		},
	}})
}

func (s *processesSuite) TestHookContextInfo(c *gc.C) {
	svc := s.env.addService(c, "proc-actions", "info-service")
	s.svc = svc
	unit := svc.deploy(c, "myproc", "xyz123", "running")

	unit.checkState(c, nil)

	args := map[string]interface{}{
		"name":   "myproc",
		"id":     "xyz123",
		"status": "running",
	}
	unit.runAction(c, "register", args)

	// checkState calls the "list" action, which wraps "info".
	unit.checkState(c, []process.Info{{
		Process: charm.Process{
			Name: "myproc",
			Type: "myplugin",
			TypeOptions: map[string]string{
				"critical": "true",
			},
			Command: "run-server",
			Image:   "web-server",
			Ports: []charm.ProcessPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "",
			}, {
				External: 8081,
				Internal: 443,
				Endpoint: "",
			}},
			Volumes: []charm.ProcessVolume{{
				ExternalMount: "/var/some-server/html",
				InternalMount: "/usr/share/some-server/html",
				Mode:          "ro",
				Name:          "",
			}, {
				ExternalMount: "/var/some-server/conf",
				InternalMount: "/etc/some-server",
				Mode:          "ro",
				Name:          "",
			}},
			EnvVars: map[string]string{
				"IMPORTANT": "some value",
			},
		},
		Status: process.Status{
			State:   process.StateRunning,
			Blocker: "",
			Message: "",
		},
		Details: process.Details{
			ID: "xyz123",
			Status: process.PluginStatus{
				State: "running",
			},
		},
	}})
}

func (s *processesSuite) TestHookContextSetStatus(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesSuite) TestHookContextUnregister(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesSuite) TestHookContextDestroy(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesSuite) TestWorkerSetStatus(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesSuite) TestWorkerCleanUp(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesSuite) TestCmdJujuStatus(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

type procsEnviron struct {
	name         string
	envType      string
	machine      string
	isolated     bool
	homedir      string
	bootstrapped bool
	failed       bool
}

func newProcsEnv(c *gc.C, envName, envType string) *procsEnviron {
	isolated := false
	if strings.HasPrefix(envType, "<") && strings.HasSuffix(envType, ">") {
		envType = envType[1 : len(envType)-1]
		isolated = true
		if envType != "local" {
			panic("not supported")
		}
	}
	if envName == "" {
		envName = envType
	}
	return &procsEnviron{
		name:     envName,
		envType:  envType,
		isolated: isolated,
	}
}

func (env *procsEnviron) markFailure(c *gc.C, s interface{}) {
	env.failed = true
}

func initJuju(c *gc.C) {
	err := jjj.InitJujuHome()
	c.Assert(err, jc.ErrorIsNil)
}

func (env *procsEnviron) ensureOSEnv(c *gc.C) {
	homedir := env.homedir
	if homedir == "" {
		if env.isolated {
			tempHomedir, err := ioutil.TempDir("", "juju-test-"+env.name+"-")
			c.Assert(err, jc.ErrorIsNil)
			env.homedir = tempHomedir
			env.writeConfig(c)
			homedir = tempHomedir
		} else {
			homedir = userInfo.HomeDir
		}
	}

	err := os.Setenv("JUJU_HOME", filepath.Join(homedir, ".juju"))
	c.Assert(err, jc.ErrorIsNil)
}

func (env procsEnviron) writeConfig(c *gc.C) {
	config := map[string]interface{}{
		"environments": map[string]interface{}{
			env.name: env.localConfig(),
		},
	}
	data, err := goyaml.Marshal(config)
	c.Assert(err, jc.ErrorIsNil)

	envYAML := filepath.Join(env.homedir, ".juju", "environments.yaml")
	err = os.MkdirAll(filepath.Dir(envYAML), 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(envYAML, data, 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (env procsEnviron) localConfig() map[string]interface{} {
	// Run in a test-only local provider
	//  (see https://github.com/juju/docs/issues/35).
	return map[string]interface{}{
		"type":         "local",
		"root-dir":     env.homedir,
		"api-port":     17071,
		"state-port":   37018,
		"storage-port": 8041,
	}
}

func (env *procsEnviron) run(c *gc.C, cmd string, args ...string) string {
	envArg := "--environment=" + env.name
	if cmd == "destroy-environment" {
		envArg = env.name
	}
	args = append([]string{envArg}, args...)
	c.Logf(" COMMAND: juju %s %s", cmd, strings.Join(args, " "))

	command := lookUpCommand(cmd)
	ctx, err := coretesting.RunCommand(c, command, args...)
	c.Assert(err, jc.ErrorIsNil)

	return strings.TrimSpace(coretesting.Stdout(ctx))
}

// TODO(ericsnow) Instead, directly access the command registry...
func lookUpCommand(cmd string) cmd.Command {
	switch cmd {
	case "bootstrap":
		return envcmd.Wrap(&commands.BootstrapCommand{})
	case "environment set":
		return envcmd.Wrap(&environment.SetCommand{})
	case "destroy-environment":
		return &commands.DestroyEnvironmentCommand{}
	case "status":
		return envcmd.Wrap(&status.StatusCommand{})
	case "add-machine":
		return envcmd.Wrap(&machine.AddCommand{})
	case "deploy":
		return envcmd.Wrap(&commands.DeployCommand{})
	case "service set":
		return envcmd.Wrap(&service.SetCommand{})
	case "service add-unit":
		return envcmd.Wrap(&service.AddUnitCommand{})
	case "destroy-service":
		return envcmd.Wrap(&commands.RemoveServiceCommand{})
	case "destroy-unit":
		return envcmd.Wrap(&commands.RemoveUnitCommand{})
	case "action do":
		return envcmd.Wrap(&action.DoCommand{})
	case "action fetch":
		return envcmd.Wrap(&action.FetchCommand{})
	default:
		panic("unknown command: " + cmd)
	}
	return nil
}

func (env *procsEnviron) bootstrap(c *gc.C) {
	if env.bootstrapped {
		return
	}
	env.ensureOSEnv(c)
	initJuju(c)

	env.run(c, "bootstrap")
	env.bootstrapped = true
	env.run(c, "environment set", "logging-config=<root>=DEBUG")
}

func (env *procsEnviron) addService(c *gc.C, charmName, serviceName string) *procsService {
	if serviceName == "" {
		serviceName = charmName
	}
	charmURL := "local:quantal/" + charmName

	if env.machine == "" {
		env.run(c, "add-machine", "--series=quantal")
		env.machine = "1"
	}
	env.run(c, "deploy", "--to="+env.machine, "--repository="+repoDir, charmURL, serviceName)
	// We leave unit /0 alive to keep the machine alive.

	svc := &procsService{
		env:       env,
		charmName: charmName,
		name:      serviceName,
	}
	svc.dummy = procsUnit{
		svc: svc,
		id:  serviceName + "/0",
	}
	return svc
}

// Set the to true to prevent the test local env from being cleaned up.
var procsDoNotCleanUp = false

func (env *procsEnviron) destroy(c *gc.C) {
	if procsDoNotCleanUp {
		return
	}
	if env.bootstrapped {
		env.ensureOSEnv(c)
		initJuju(c)
		env.run(c, "destroy-environment", "--force")
		env.bootstrapped = false
	}
	if env.homedir != "" {
		err := os.RemoveAll(env.homedir)
		c.Assert(err, jc.ErrorIsNil)
	}
}

type procsService struct {
	env       *procsEnviron
	charmName string
	name      string
	dummy     procsUnit
	lastUnit  int
}

func (svc *procsService) block(c *gc.C) {
	svc.dummy.runAction(c, "noop", nil)
}

func (svc *procsService) setConfig(c *gc.C, settings map[string]string) {
	if len(settings) == 0 {
		return
	}
	// Try to force at least a little sequentiality on Juju.
	svc.block(c)

	args := []string{svc.name}
	for k, v := range settings {
		args = append(args, fmt.Sprintf("%s=%s", k, v))
	}
	svc.env.run(c, "service set", args...)
	//filename := procsYAMLFile(c, map[string]interface{}{svc.name: settings})
	//svc.env.run(c, "service set", "--config="+filename, svc.name)
}

func (svc *procsService) deploy(c *gc.C, procName, pluginID, status string) *procsUnit {
	settings := map[string]string{
		"plugin-name":   procName,
		"plugin-id":     pluginID,
		"plugin-status": status,
	}
	svc.setConfig(c, settings)

	svc.env.run(c, "service add-unit", "--to="+svc.env.machine, svc.name)

	svc.lastUnit += 1
	u := &procsUnit{
		svc: svc,
		id:  fmt.Sprintf("%s/%d", svc.name, svc.lastUnit),
	}

	u.waitForStatus(c, "started", "pending")
	return u
}

func (svc *procsService) destroy(c *gc.C) {
	if procsDoNotCleanUp {
		return
	}
	svc.env.run(c, "destroy-service", svc.name)
}

type procsUnit struct {
	svc *procsService
	id  string
}

func (u *procsUnit) waitForStatus(c *gc.C, target string, okayList ...string) {
	// TODO(ericsnow) Support a timeout?
	for {
		status := u.agentStatus(c)
		if status == target {
			return
		}
		invalid := true
		for _, okay := range okayList {
			if status == okay {
				invalid = false
				break
			}
		}
		if invalid {
			c.Errorf("invalid status %q", status)
			c.FailNow()
		}
	}
}

var procsStatusRegex = regexp.MustCompile(`^- (.*): .* \((.*)\)$`)

func (u *procsUnit) agentStatus(c *gc.C) string {
	out := u.svc.env.run(c, "status", "--format=short")
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		match := procsStatusRegex.FindStringSubmatch(line)
		if match[0] == "" {
			// Not a match.
			continue
		}
		unitName, status := match[1], match[2]
		if unitName == u.id {
			return status
		}
	}
	c.Errorf("no status found for %q", u.id)
	c.FailNow()
	return ""
}

func (u *procsUnit) setConfigStatus(c *gc.C, status string) {
	settings := map[string]string{"plugin-status": status}
	u.svc.setConfig(c, settings)
}

func (u *procsUnit) destroy(c *gc.C) {
	u.svc.env.run(c, "destroy-unit", u.id)

	u.waitForStatus(c, "stopped", "started")
}

func (u *procsUnit) runAction(c *gc.C, action string, actionArgs map[string]interface{}) map[string]string {
	// Send the command.
	args := []string{
		u.id,
		action,
	}
	for k, v := range actionArgs {
		args = append(args, fmt.Sprintf("%s=%s", k, v))
	}
	doOut := u.svc.env.run(c, "action do", args...)
	//filename := procsYAMLFile(c, actionArgs)
	//doOut := u.svc.env.run(c, "action do", "--params="+filename, u.id, action)
	c.Assert(strings.Split(doOut, ": "), gc.HasLen, 2)
	actionID := strings.Split(doOut, ": ")[1]

	// Get the results.
	fetchOut := u.svc.env.run(c, "action fetch", "--wait=0", actionID)
	result := struct {
		Status  string
		Results map[string]interface{}
	}{}
	err := goyaml.Unmarshal([]byte(fetchOut), &result)
	c.Assert(err, jc.ErrorIsNil)

	// Check and coerce the results.
	if !c.Check(result.Status, gc.Equals, "completed") {
		c.Logf(" got:\n" + fetchOut)
	}
	results := make(map[string]string, len(result.Results))
	for k, v := range result.Results {
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

	results := u.runAction(c, "list", nil)
	out, ok := results["output"]
	c.Assert(ok, jc.IsTrue)
	if strings.TrimSpace(out) != "[no processes registered]" {
		procsMap := make(map[string]process.Info)
		err := goyaml.Unmarshal([]byte(out), &procsMap)
		c.Assert(err, jc.ErrorIsNil)
		for _, proc := range procsMap {
			procs = append(procs, proc)
		}
	}

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

func procsYAMLFile(c *gc.C, value interface{}) string {
	filename := filepath.Join(c.MkDir(), "data.yaml")
	data, err := goyaml.Marshal(value)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filename, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)
	return filename
}
