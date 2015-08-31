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
	"time"

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
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/workers"
)

func initWorkloadsSuites() {
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
	// TODO(ericsnow) Re-enable once we get the local provider cleaning
	// up properly.
	// gc.Suite(&workloadsSuite{})
}

var (
	repoDir  = testcharms.Repo.Path()
	userInfo *user.User
	// Set this to true to prevent the test env from being cleaned up.
	workloadsDoNotCleanUp = false
)

type workloadsSuite struct {
	env *workloadsEnviron
	svc *workloadsService
}

func (s *workloadsSuite) SetUpSuite(c *gc.C) {
	env := newWorkloadsEnv(c, "", "<local>")
	env.bootstrap(c)
	s.env = env
}

func (s *workloadsSuite) TearDownSuite(c *gc.C) {
	if s.env != nil {
		s.env.destroy(c)
	}
}

func (s *workloadsSuite) SetUpTest(c *gc.C) {
}

func (s *workloadsSuite) TearDownTest(c *gc.C) {
	if c.Failed() {
		s.env.markFailure(c, s)
	} else {
		if s.svc != nil {
			s.svc.destroy(c)
		}
	}
}

func (s *workloadsSuite) TestHookLifecycle(c *gc.C) {
	// start: info, launch, info
	// config-changed: info, set-status, info
	// stop: info, destroy, info

	svc := s.env.addService(c, "workload-hooks", "a-service")
	s.svc = svc

	// Add/start the unit.

	unit := svc.deploy(c, "myworkload", "xyz123", "running")

	unit.checkState(c, []workload.Info{{
		Workload: charm.Workload{
			Name: "myworkload",
			Type: "myplugin",
			TypeOptions: map[string]string{
				"critical": "true",
			},
			Command: "run-server",
			Image:   "web-server",
			Ports: []charm.WorkloadPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "",
			}, {
				External: 8081,
				Internal: 443,
				Endpoint: "",
			}},
			Volumes: []charm.WorkloadVolume{{
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
		Status: workload.Status{
			State:   workload.StateRunning,
			Blocker: "",
			Message: "myworkload/xyz123 is being tracked",
		},
		Details: workload.Details{
			ID: "xyz123",
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}})
	unit.checkPluginLog(c, []string{
		`myworkload	xyz123	running		added`,
		`myworkload	xyz123	running	{"Name":"myworkload","Description":"","Type":"myplugin","TypeOptions":{"critical":"true"},"Command":"run-server","Image":"web-server","Ports":[{"External":8080,"Internal":80,"Endpoint":""},{"External":8081,"Internal":443,"Endpoint":""}],"Volumes":[{"ExternalMount":"/var/some-server/html","InternalMount":"/usr/share/some-server/html","Mode":"ro","Name":""},{"ExternalMount":"/var/some-server/conf","InternalMount":"/etc/some-server","Mode":"ro","Name":""}],"EnvVars":{"IMPORTANT":"some value"}}	definition set`,
	})

	// Change the config.

	// TODO(ericsnow) Implement once workload-set-status exists...

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

func (s *workloadsSuite) TestHookContextTrack(c *gc.C) {
	svc := s.env.addService(c, "workload-actions", "track-service")
	s.svc = svc

	args := map[string]interface{}{
		"name":   "myworkload",
		"id":     "xyz123",
		"status": "running",
	}
	svc.dummy.runAction(c, "track", args)

	svc.dummy.checkState(c, []workload.Info{{
		Workload: charm.Workload{
			Name: "myworkload",
			Type: "myplugin",
			TypeOptions: map[string]string{
				"critical": "true",
			},
			Command: "run-server",
			Image:   "web-server",
			Ports: []charm.WorkloadPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "",
			}, {
				External: 8081,
				Internal: 443,
				Endpoint: "",
			}},
			Volumes: []charm.WorkloadVolume{{
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
		Status: workload.Status{
			State:   workload.StateRunning,
			Blocker: "",
			Message: "myworkload/xyz123 is being tracked",
		},
		Details: workload.Details{
			ID: "xyz123",
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}})
}

func (s *workloadsSuite) TestHookContextLaunch(c *gc.C) {
	svc := s.env.addService(c, "workload-actions", "launch-service")
	s.svc = svc

	svc.dummy.prepPlugin(c, "myworkload", "xyz123", "running")

	args := map[string]interface{}{
		"name": "myworkload",
	}
	svc.dummy.runAction(c, "launch", args)

	svc.dummy.checkState(c, []workload.Info{{
		Workload: charm.Workload{
			Name: "myworkload",
			Type: "myplugin",
			TypeOptions: map[string]string{
				"critical": "true",
			},
			Command: "run-server",
			Image:   "web-server",
			Ports: []charm.WorkloadPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "",
			}, {
				External: 8081,
				Internal: 443,
				Endpoint: "",
			}},
			Volumes: []charm.WorkloadVolume{{
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
		Status: workload.Status{
			State:   workload.StateRunning,
			Blocker: "",
			Message: "myworkload/xyz123 is being tracked",
		},
		Details: workload.Details{
			ID: "xyz123",
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}})
}

func (s *workloadsSuite) TestHookContextInfo(c *gc.C) {
	svc := s.env.addService(c, "workload-actions", "info-service")
	s.svc = svc
	unit := svc.deploy(c, "myworkload", "xyz123", "running")

	unit.checkState(c, nil)

	args := map[string]interface{}{
		"name":   "myworkload",
		"id":     "xyz123",
		"status": "running",
	}
	unit.runAction(c, "track", args)

	// checkState calls the "list" action, which wraps "info".
	unit.checkState(c, []workload.Info{{
		Workload: charm.Workload{
			Name: "myworkload",
			Type: "myplugin",
			TypeOptions: map[string]string{
				"critical": "true",
			},
			Command: "run-server",
			Image:   "web-server",
			Ports: []charm.WorkloadPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "",
			}, {
				External: 8081,
				Internal: 443,
				Endpoint: "",
			}},
			Volumes: []charm.WorkloadVolume{{
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
		Status: workload.Status{
			State:   workload.StateRunning,
			Blocker: "",
			Message: "myworkload/xyz123 is being tracked",
		},
		Details: workload.Details{
			ID: "xyz123",
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}})
}

func (s *workloadsSuite) TestHookContextSetStatus(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *workloadsSuite) TestHookContextUntrack(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *workloadsSuite) TestHookContextDestroy(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *workloadsSuite) TestWorkerSetStatus(c *gc.C) {
	svc := s.env.addService(c, "workload-actions", "workerStatusSvc")
	s.svc = svc

	svc.dummy.prepPlugin(c, "myworkload", "xyz123", "running")

	args := map[string]interface{}{
		"name": "myworkload",
	}
	period := time.Second * 5
	workers.SetStatusWorkerUpdatePeriod(period)
	svc.dummy.runAction(c, "launch", args)

	args = map[string]interface{}{
		"id":     "xyz123",
		"status": "testing",
	}
	svc.dummy.runAction(c, "plugin-setstatus", args)
	time.Sleep(period * 2)

	svc.dummy.checkState(c, []workload.Info{{
		Workload: charm.Workload{
			Name: "myworkload",
			Type: "myplugin",
			TypeOptions: map[string]string{
				"critical": "true",
			},
			Command: "run-server",
			Image:   "web-server",
			Ports: []charm.WorkloadPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "",
			}, {
				External: 8081,
				Internal: 443,
				Endpoint: "",
			}},
			Volumes: []charm.WorkloadVolume{{
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
		Status: workload.Status{
			State:   workload.StateRunning,
			Blocker: "",
			Message: "myworkload/xyz123 is being tracked",
		},
		Details: workload.Details{
			ID: "xyz123",
			Status: workload.PluginStatus{
				State: "testing",
			},
		},
	}})

}

func (s *workloadsSuite) TestWorkerCleanUp(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *workloadsSuite) TestCmdJujuStatus(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

type workloadsEnviron struct {
	name         string
	envType      string
	machine      string
	isolated     bool
	homedir      string
	bootstrapped bool
	failed       bool
}

func newWorkloadsEnv(c *gc.C, envName, envType string) *workloadsEnviron {
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
	return &workloadsEnviron{
		name:     envName,
		envType:  envType,
		isolated: isolated,
	}
}

func (env *workloadsEnviron) markFailure(c *gc.C, s interface{}) {
	env.failed = true
}

func initJuju(c *gc.C) {
	err := jjj.InitJujuHome()
	c.Assert(err, jc.ErrorIsNil)
}

func (env *workloadsEnviron) ensureOSEnv(c *gc.C) {
	homedir := env.homedir
	if homedir == "" {
		if env.isolated {
			tempHomedir, err := ioutil.TempDir("", "juju-test-"+env.name+"-")
			c.Assert(err, jc.ErrorIsNil)
			env.homedir = tempHomedir

			rootdir := filepath.Join(env.homedir, "rootdir")
			err = os.MkdirAll(filepath.Dir(rootdir), 0755)
			c.Assert(err, jc.ErrorIsNil)

			env.writeConfig(c, rootdir)
			homedir = tempHomedir
		} else {
			homedir = userInfo.HomeDir
		}
	}

	err := os.Setenv("JUJU_HOME", filepath.Join(homedir, ".juju"))
	c.Assert(err, jc.ErrorIsNil)
}

func (env workloadsEnviron) writeConfig(c *gc.C, rootdir string) {
	config := map[string]interface{}{
		"environments": map[string]interface{}{
			env.name: env.localConfig(rootdir),
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

func (env workloadsEnviron) localConfig(rootdir string) map[string]interface{} {
	// Run in a test-only local provider
	//  (see https://github.com/juju/docs/issues/35).
	return map[string]interface{}{
		"type":         "local",
		"root-dir":     rootdir,
		"api-port":     17071,
		"state-port":   37018,
		"storage-port": 8041,
	}
}

func (env *workloadsEnviron) run(c *gc.C, cmd string, args ...string) string {
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

func (env *workloadsEnviron) bootstrap(c *gc.C) {
	if env.bootstrapped {
		return
	}
	env.ensureOSEnv(c)
	initJuju(c)

	env.run(c, "bootstrap")
	env.bootstrapped = true
	env.run(c, "environment set", "logging-config=<root>=DEBUG")
}

func (env *workloadsEnviron) addService(c *gc.C, charmName, serviceName string) *workloadsService {
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

	svc := &workloadsService{
		env:       env,
		charmName: charmName,
		name:      serviceName,
	}
	svc.dummy = workloadsUnit{
		svc: svc,
		id:  serviceName + "/0",
	}
	return svc
}

func (env *workloadsEnviron) destroy(c *gc.C) {
	if workloadsDoNotCleanUp {
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

type workloadsService struct {
	env       *workloadsEnviron
	charmName string
	name      string
	dummy     workloadsUnit
	lastUnit  int
}

func (svc *workloadsService) block(c *gc.C) {
	svc.dummy.runAction(c, "noop", nil)
}

func (svc *workloadsService) setConfig(c *gc.C, settings map[string]string) {
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
	//filename := workloadsYAMLFile(c, map[string]interface{}{svc.name: settings})
	//svc.env.run(c, "service set", "--config="+filename, svc.name)
}

func (svc *workloadsService) deploy(c *gc.C, workloadName, pluginID, status string) *workloadsUnit {
	settings := map[string]string{
		"plugin-name":   workloadName,
		"plugin-id":     pluginID,
		"plugin-status": status,
	}
	svc.setConfig(c, settings)

	svc.env.run(c, "service add-unit", "--to="+svc.env.machine, svc.name)

	svc.lastUnit += 1
	u := &workloadsUnit{
		svc: svc,
		id:  fmt.Sprintf("%s/%d", svc.name, svc.lastUnit),
	}

	u.waitForStatus(c, "started", "pending")
	return u
}

func (svc *workloadsService) destroy(c *gc.C) {
	if workloadsDoNotCleanUp {
		return
	}
	svc.env.run(c, "destroy-service", svc.name)
}

type workloadsUnit struct {
	svc *workloadsService
	id  string
}

func (u *workloadsUnit) waitForStatus(c *gc.C, target string, okayList ...string) {
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

var workloadsStatusRegex = regexp.MustCompile(`^- (.*): .* \((.*)\)$`)

func (u *workloadsUnit) agentStatus(c *gc.C) string {
	out := u.svc.env.run(c, "status", "--format=short")
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		match := workloadsStatusRegex.FindStringSubmatch(line)
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

func (u *workloadsUnit) setConfigStatus(c *gc.C, status string) {
	settings := map[string]string{"plugin-status": status}
	u.svc.setConfig(c, settings)
}

func (u *workloadsUnit) destroy(c *gc.C) {
	u.svc.env.run(c, "destroy-unit", u.id)

	u.waitForStatus(c, "stopped", "started")
}

func (u *workloadsUnit) runAction(c *gc.C, action string, actionArgs map[string]interface{}) map[string]string {
	// Send the command.
	args := []string{
		u.id,
		action,
	}
	for k, v := range actionArgs {
		args = append(args, fmt.Sprintf("%s=%s", k, v))
	}
	doOut := u.svc.env.run(c, "action do", args...)
	//filename := workloadsYAMLFile(c, actionArgs)
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

func (u *workloadsUnit) injectStatus(c *gc.C, pluginID, status string) {
	args := map[string]interface{}{
		"id":     pluginID,
		"status": status,
	}
	u.runAction(c, "plugin-setstatus", args)
}

func (u *workloadsUnit) prepPlugin(c *gc.C, workloadName, pluginID, status string) {
	args := map[string]interface{}{
		"name":   workloadName,
		"id":     pluginID,
		"status": status,
	}
	u.runAction(c, "plugin-prep", args)
}

func (u *workloadsUnit) checkState(c *gc.C, expected []workload.Info) {
	var workloads []workload.Info

	results := u.runAction(c, "list", nil)
	out, ok := results["output"]
	c.Assert(ok, jc.IsTrue)
	if strings.TrimSpace(out) != "[no workloads tracked]" {
		workloadsMap := make(map[string]workload.Info)
		err := goyaml.Unmarshal([]byte(out), &workloadsMap)
		c.Assert(err, jc.ErrorIsNil)
		for _, wl := range workloadsMap {
			workloads = append(workloads, wl)
		}
	}

	c.Check(workloads, jc.DeepEquals, expected)
}

func (u *workloadsUnit) checkPluginLog(c *gc.C, expected []string) {
	results := u.runAction(c, "plugin-dump", nil)
	c.Assert(results, gc.HasLen, 1)
	output, ok := results["output"]
	c.Assert(ok, jc.IsTrue)
	lines := strings.Split(output, "\n")

	c.Check(lines, jc.DeepEquals, expected)
}

func workloadsYAMLFile(c *gc.C, value interface{}) string {
	filename := filepath.Join(c.MkDir(), "data.yaml")
	data, err := goyaml.Marshal(value)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filename, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)
	return filename
}
