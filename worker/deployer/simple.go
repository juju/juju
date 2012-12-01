package deployer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/version"
)

// SimpleContext is a Context that manages unit deployments via upstart
// jobs on the local system.
type SimpleContext struct {

	// StateInfo identifies (by EntityName) the agent responsible for
	// deployments in this context, and (by Addrs and CACert) the
	// common information required to connect a new agent to state.
	StateInfo *state.Info

	// InitDir specifies the directory used by upstart on the local system.
	// It is typically set to "/etc/init".
	InitDir string

	// DataDir specifies the directory used by juju to store its state. It
	// is typically set to "/var/lib/juju".
	DataDir string

	// LogDir specifies the directory to which installed units will write
	// their log files. It is typically set to "/var/log/juju".
	LogDir string
}

var _ Context = &SimpleContext{}

// NewSimpleContext returns a new SimpleContext, acting on behalf of the
// entity specified in info, that deploys unit agents as upstart jobs in
// "/etc/init" logging to "/var/log/juju". Paths to which agents and tools
// are installed are relative to dataDir; if dataDir is empty, it will be
// set to "/var/lib/juju".
func NewSimpleContext(info *state.Info, dataDir string) *SimpleContext {
	if dataDir == "" {
		dataDir = "/var/lib/juju"
	}
	return &SimpleContext{
		StateInfo: info,
		InitDir:   "/etc/init",
		DataDir:   dataDir,
		LogDir:    "/var/log/juju",
	}
}

func (ctx *SimpleContext) DeployerName() string {
	return ctx.StateInfo.EntityName
}

func (ctx *SimpleContext) DeployUnit(name, initialPassword string) (err error) {
	// Check sanity.
	svc := ctx.upstartService(name)
	if svc.Installed() {
		return fmt.Errorf("unit %q is already deployed", name)
	}

	// Link the current tools for use by the new agent.
	entityName := state.UnitEntityName(name)
	_, err = environs.ChangeAgentTools(ctx.DataDir, entityName, version.Current)
	toolsDir := environs.AgentToolsDir(ctx.DataDir, entityName)
	defer removeOnErr(&err, toolsDir)

	// Create the agent's state directory.
	agentDir := environs.AgentDir(ctx.DataDir, entityName)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return err
	}
	defer removeOnErr(&err, agentDir)

	// Create the CA certificate used to validate the state connection.
	certPath := filepath.Join(agentDir, "ca-cert.pem")
	if err := ioutil.WriteFile(certPath, ctx.StateInfo.CACert, 0644); err != nil {
		return err
	}
	defer removeOnErr(&err, certPath)

	// Install an upstart job that runs the unit agent.
	logPath := filepath.Join(ctx.LogDir, entityName+".log")
	cmd := strings.Join([]string{
		filepath.Join(toolsDir, "jujud"), "unit",
		"--unit-name", name,
		"--ca-cert", certPath,
		"--state-servers", strings.Join(ctx.StateInfo.Addrs, ","),
		"--initial-password", initialPassword,
		"--log-file", logPath,
		"--debug", // TODO: propagate debug state sensibly
	}, " ")
	conf := &upstart.Conf{
		Service: *svc,
		Desc:    "juju unit agent for " + name,
		Cmd:     cmd,
		Out:     logPath,
	}
	return conf.Install()
}

func (ctx *SimpleContext) RecallUnit(name string) error {
	svc := ctx.upstartService(name)
	if !svc.Installed() {
		return fmt.Errorf("unit %q is not deployed", name)
	}
	if err := svc.Remove(); err != nil {
		return err
	}
	entityName := state.UnitEntityName(name)
	agentDir := environs.AgentDir(ctx.DataDir, entityName)
	if err := os.RemoveAll(agentDir); err != nil {
		return err
	}
	toolsDir := environs.AgentToolsDir(ctx.DataDir, entityName)
	return os.Remove(toolsDir)
}

func (ctx *SimpleContext) DeployedUnits() ([]string, error) {
	pattern := "^jujud-unit-([a-z0-9-]+)-([0-9]+)-x-" + ctx.DeployerName() + "\\.conf$"
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	fis, err := ioutil.ReadDir(ctx.InitDir)
	if err != nil {
		return nil, err
	}
	var installed []string
	for _, fi := range fis {
		if groups := re.FindStringSubmatch(fi.Name()); len(groups) == 3 {
			name := groups[1] + "/" + groups[2]
			if !state.IsUnitName(name) {
				continue
			}
			installed = append(installed, name)
		}
	}
	return installed, nil
}

// upstartService returns an upstart.Service corresponding to the specified
// unit. Its name is badged according to the entity responsible for the
// context, so as to distinguish its own jobs from those installed by other
// means.
func (ctx *SimpleContext) upstartService(name string) *upstart.Service {
	entityName := state.UnitEntityName(name)
	svcName := "jujud-" + entityName + "-x-" + ctx.DeployerName()
	svc := upstart.NewService(svcName)
	svc.InitDir = ctx.InitDir
	return svc
}

func removeOnErr(err *error, path string) {
	if *err != nil {
		if e := os.Remove(path); e != nil {
			log.Printf("installer: cannot remove %q: %v", path, e)
		}
	}
}
