// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver/params"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/version"
)

// InitDir is the default upstart init directory.
// This is a var so it can be overridden by tests.
var InitDir = "/etc/init"

// APICalls defines the interface to the API that the simple context needs.
type APICalls interface {
	ConnectionInfo() (params.DeployerConnectionValues, error)
}

// SimpleContext is a Context that manages unit deployments on the local system.
type SimpleContext struct {

	// api is used to get the current state server addresses at the time the
	// given unit is deployed.
	api APICalls

	// agentConfig returns the agent config for the machine agent that is
	// running the deployer.
	agentConfig agent.Config

	// initDir specifies the directory used by upstart on the local system.
	// It is typically set to "/etc/init".
	initDir string
}

var _ Context = (*SimpleContext)(nil)

// recursiveChmod will change the permissions on all files and
// folders inside path
func recursiveChmod(path string, mode os.FileMode) error {
	walker := func(p string, fi os.FileInfo, err error) error {
		if _, err := os.Stat(p); err == nil {
			errPerm := os.Chmod(p, mode)
			if errPerm != nil {
				return errPerm
			}
		}
		return nil
	}
	if err := filepath.Walk(path, walker); err != nil {
		return err
	}
	return nil
}

// NewSimpleContext returns a new SimpleContext, acting on behalf of
// the specified deployer, that deploys unit agents.
// Paths to which agents and tools are installed are relative to dataDir.
func NewSimpleContext(agentConfig agent.Config, api APICalls) *SimpleContext {
	return &SimpleContext{
		api:         api,
		agentConfig: agentConfig,
		initDir:     InitDir,
	}
}

func (ctx *SimpleContext) AgentConfig() agent.Config {
	return ctx.agentConfig
}

func (ctx *SimpleContext) DeployUnit(unitName, initialPassword string) (err error) {
	// Check sanity.
	svc := ctx.service(unitName)
	if svc.Installed() {
		return fmt.Errorf("unit %q is already deployed", unitName)
	}

	// Link the current tools for use by the new agent.
	tag := names.NewUnitTag(unitName)
	dataDir := ctx.agentConfig.DataDir()
	logDir := ctx.agentConfig.LogDir()
	// TODO(dfc)
	_, err = tools.ChangeAgentTools(dataDir, tag.String(), version.Current)
	// TODO(dfc)
	toolsDir := tools.ToolsDir(dataDir, tag.String())
	defer removeOnErr(&err, toolsDir)

	result, err := ctx.api.ConnectionInfo()
	if err != nil {
		return err
	}
	logger.Debugf("state addresses: %q", result.StateAddresses)
	logger.Debugf("API addresses: %q", result.APIAddresses)
	containerType := ctx.agentConfig.Value(agent.ContainerType)
	namespace := ctx.agentConfig.Value(agent.Namespace)
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			DataDir:           dataDir,
			LogDir:            logDir,
			UpgradedToVersion: version.Current.Number,
			Tag:               tag,
			Password:          initialPassword,
			Nonce:             "unused",
			// TODO: remove the state addresses here and test when api only.
			StateAddresses: result.StateAddresses,
			APIAddresses:   result.APIAddresses,
			CACert:         ctx.agentConfig.CACert(),
			Values: map[string]string{
				agent.ContainerType: containerType,
				agent.Namespace:     namespace,
			},
		})
	if err != nil {
		return err
	}
	if err := conf.Write(); err != nil {
		return err
	}
	defer removeOnErr(&err, conf.Dir())

	// Install an upstart job that runs the unit agent.
	logPath := path.Join(logDir, tag.String()+".log")
	cmd := strings.Join([]string{
		filepath.FromSlash(path.Join(toolsDir, jujunames.Jujud)), "unit",
		"--data-dir", dataDir,
		"--unit-name", unitName,
		"--debug", // TODO: propagate debug state sensibly
	}, " ")
	// TODO(thumper): 2013-09-02 bug 1219630
	// As much as I'd like to remove JujuContainerType now, it is still
	// needed as MAAS still needs it at this stage, and we can't fix
	// everything at once.
	sconf := common.Conf{
		Desc: "juju unit agent for " + unitName,
		Cmd:  cmd,
		Out:  logPath,
		Env: map[string]string{
			osenv.JujuContainerTypeEnvKey: containerType,
		},
		InitDir: ctx.initDir,
	}
	svc.UpdateConfig(sconf)
	return svc.Install()
}

// findUpstartJob tries to find an upstart job matching the
// given unit name in one of these formats:
//   jujud-<deployer-tag>:<unit-tag>.conf (for compatibility)
//   jujud-<unit-tag>.conf (default)
func (ctx *SimpleContext) findUpstartJob(unitName string) service.Service {
	unitsAndJobs, err := ctx.deployedUnitsUpstartJobs()
	if err != nil {
		return nil
	}
	if job, ok := unitsAndJobs[unitName]; ok {
		svc := service.NewService(job, common.Conf{InitDir: ctx.initDir})
		return svc
	}
	return nil
}

func (ctx *SimpleContext) RecallUnit(unitName string) error {
	svc := ctx.findUpstartJob(unitName)
	if svc == nil || !svc.Installed() {
		return fmt.Errorf("unit %q is not deployed", unitName)
	}
	if err := svc.StopAndRemove(); err != nil {
		return err
	}
	tag := names.NewUnitTag(unitName)
	dataDir := ctx.agentConfig.DataDir()
	agentDir := agent.Dir(dataDir, tag)
	// Recursivley change mode to 777 on windows to avoid
	// Operation not permitted errors when deleting the agentDir
	err := recursiveChmod(agentDir, os.FileMode(0777))
	if err != nil {
		return err
	}
	if err := os.RemoveAll(agentDir); err != nil {
		return err
	}
	// TODO(dfc) should take a Tag
	toolsDir := tools.ToolsDir(dataDir, tag.String())
	return os.Remove(toolsDir)
}

var deployedRe = regexp.MustCompile("^(jujud-.*unit-([a-z0-9-]+)-([0-9]+))$")

func (ctx *SimpleContext) deployedUnitsUpstartJobs() (map[string]string, error) {
	fis, err := service.ListServices(ctx.initDir)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	installed := make(map[string]string)
	for _, fi := range fis {
		if groups := deployedRe.FindStringSubmatch(fi); len(groups) > 0 {
			unitName := groups[2] + "/" + groups[3]
			if !names.IsValidUnit(unitName) {
				continue
			}
			installed[unitName] = groups[1]
		}
	}
	return installed, nil
}

func (ctx *SimpleContext) DeployedUnits() ([]string, error) {
	unitsAndJobs, err := ctx.deployedUnitsUpstartJobs()
	if err != nil {
		return nil, err
	}
	var installed []string
	for unitName := range unitsAndJobs {
		installed = append(installed, unitName)
	}
	return installed, nil
}

// service returns a service.Service corresponding to the specified
// unit.
func (ctx *SimpleContext) service(unitName string) service.Service {
	tag := names.NewUnitTag(unitName).String()
	svcName := "jujud-" + tag
	svc := service.NewService(svcName, common.Conf{InitDir: ctx.initDir})
	return svc
}

func removeOnErr(err *error, path string) {
	if *err != nil {
		if err := os.Remove(path); err != nil {
			logger.Warningf("installer: cannot remove %q: %v", path, err)
		}
	}
}
