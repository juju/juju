package deployer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/version"
)

// SimpleManager is a Manager that manages unit deployments via upstart
// jobs on the local system.
type SimpleManager struct {

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

var _ Manager = (*SimpleManager)(nil)

// NewSimpleManager returns a new SimpleManager, acting on behalf of the
// entity specified in info, that deploys unit agents as upstart jobs in
// "/etc/init" logging to "/var/log/juju". Paths to which agents and tools
// are installed are relative to dataDir; if dataDir is empty, it will be
// set to "/var/lib/juju".
func NewSimpleManager(info *state.Info, dataDir string) *SimpleManager {
	if dataDir == "" {
		dataDir = "/var/lib/juju"
	}
	return &SimpleManager{
		StateInfo: info,
		InitDir:   "/etc/init",
		DataDir:   dataDir,
		LogDir:    "/var/log/juju",
	}
}

func (mgr *SimpleManager) DeployUnit(unitName, initialPassword string) (err error) {
	// Check sanity.
	svc := mgr.upstartService(unitName)
	if svc.Installed() {
		return fmt.Errorf("unit %q is already deployed", unitName)
	}

	// Link the current tools for use by the new agent.
	entityName := state.UnitEntityName(unitName)
	_, err = environs.ChangeAgentTools(mgr.DataDir, entityName, version.Current)
	toolsDir := environs.AgentToolsDir(mgr.DataDir, entityName)
	defer removeOnErr(&err, toolsDir)

	// Prepare the agent's configuration data.
	conf := &agent.Conf{
		DataDir:     mgr.DataDir,
		OldPassword: initialPassword,
		StateInfo:   *mgr.StateInfo,
	}
	conf.StateInfo.EntityName = entityName
	conf.StateInfo.Password = ""
	if err := conf.Write(); err != nil {
		return err
	}
	defer removeOnErr(&err, conf.Dir())

	// Install an upstart job that runs the unit agent.
	logPath := filepath.Join(mgr.LogDir, entityName+".log")
	cmd := strings.Join([]string{
		filepath.Join(toolsDir, "jujud"), "unit",
		"--data-dir", conf.DataDir,
		"--unit-name", unitName,
		"--debug", // TODO: propagate debug state sensibly
	}, " ")
	uconf := &upstart.Conf{
		Service: *svc,
		Desc:    "juju unit agent for " + unitName,
		Cmd:     cmd,
		Out:     logPath,
	}
	return uconf.Install()
}

func (mgr *SimpleManager) RecallUnit(unitName string) error {
	svc := mgr.upstartService(unitName)
	if !svc.Installed() {
		return fmt.Errorf("unit %q is not deployed", unitName)
	}
	if err := svc.Remove(); err != nil {
		return err
	}
	entityName := state.UnitEntityName(unitName)
	agentDir := environs.AgentDir(mgr.DataDir, entityName)
	if err := os.RemoveAll(agentDir); err != nil {
		return err
	}
	toolsDir := environs.AgentToolsDir(mgr.DataDir, entityName)
	return os.Remove(toolsDir)
}

var deployedRe = regexp.MustCompile("^jujud-([a-z0-9-]+):unit-([a-z0-9-]+)-([0-9]+)\\.conf$")

func (mgr *SimpleManager) DeployedUnits() ([]string, error) {
	fis, err := ioutil.ReadDir(mgr.InitDir)
	if err != nil {
		return nil, err
	}
	var installed []string
	for _, fi := range fis {
		if groups := deployedRe.FindStringSubmatch(fi.Name()); len(groups) == 4 {
			if groups[1] != mgr.StateInfo.EntityName {
				continue
			}
			unitName := groups[2] + "/" + groups[3]
			if !state.IsUnitName(unitName) {
				continue
			}
			installed = append(installed, unitName)
		}
	}
	return installed, nil
}

// upstartService returns an upstart.Service corresponding to the specified
// unit. Its name is badged according to the entity responsible for the
// context, so as to distinguish its own jobs from those installed by other
// means.
func (mgr *SimpleManager) upstartService(unitName string) *upstart.Service {
	entityName := state.UnitEntityName(unitName)
	svcName := "jujud-" + mgr.StateInfo.EntityName + ":" + entityName
	svc := upstart.NewService(svcName)
	svc.InitDir = mgr.InitDir
	return svc
}

func removeOnErr(err *error, path string) {
	if *err != nil {
		if e := os.Remove(path); e != nil {
			log.Printf("installer: cannot remove %q: %v", path, e)
		}
	}
}
