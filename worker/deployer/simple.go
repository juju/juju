// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/version"
)

// TODO(ericsnow) Use errors.Trace, etc. in this file.

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

	// discoverService is a surrogate for service.DiscoverService.
	discoverService func(string, common.Conf) (deployerService, error)

	// listServices is a surrogate for service.ListServices.
	listServices func() ([]string, error)
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
		discoverService: func(name string, conf common.Conf) (deployerService, error) {
			return service.DiscoverService(name, conf)
		},
		listServices: func() ([]string, error) {
			return service.ListServices()
		},
	}
}

func (ctx *SimpleContext) AgentConfig() agent.Config {
	return ctx.agentConfig
}

func (ctx *SimpleContext) DeployUnit(unitName, initialPassword string) (err error) {
	// Check sanity.
	renderer, err := shell.NewRenderer("")
	if err != nil {
		return errors.Trace(err)
	}
	svc, err := ctx.service(unitName, renderer)
	if err != nil {
		return errors.Trace(err)
	}
	installed, err := svc.Installed()
	if err != nil {
		return errors.Trace(err)
	}
	if installed {
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
			Environment:       ctx.agentConfig.Environment(),
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

	// Install an init service that runs the unit agent.
	if err := service.InstallAndStart(svc); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type deployerService interface {
	Installed() (bool, error)
	Install() error
	Remove() error
	Start() error
	Stop() error
}

// findUpstartJob tries to find an init system job matching the
// given unit name in one of these formats:
//   jujud-<deployer-tag>:<unit-tag>.conf (for compatibility)
//   jujud-<unit-tag>.conf (default)
func (ctx *SimpleContext) findInitSystemJob(unitName string) (deployerService, error) {
	unitsAndJobs, err := ctx.deployedUnitsInitSystemJobs()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if job, ok := unitsAndJobs[unitName]; ok {
		return ctx.discoverService(job, common.Conf{})
	}
	return nil, errors.Errorf("unit %q is not deployed", unitName)
}

func (ctx *SimpleContext) RecallUnit(unitName string) error {
	svc, err := ctx.findInitSystemJob(unitName)
	if err != nil {
		return errors.Trace(err)
	}
	installed, err := svc.Installed()
	if err != nil {
		return errors.Trace(err)
	}
	if !installed {
		return errors.Errorf("unit %q is not deployed", unitName)
	}
	if err := svc.Stop(); err != nil {
		return err
	}
	if err := svc.Remove(); err != nil {
		return err
	}
	tag := names.NewUnitTag(unitName)
	dataDir := ctx.agentConfig.DataDir()
	agentDir := agent.Dir(dataDir, tag)
	// Recursivley change mode to 777 on windows to avoid
	// Operation not permitted errors when deleting the agentDir
	err = recursiveChmod(agentDir, os.FileMode(0777))
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

func (ctx *SimpleContext) deployedUnitsInitSystemJobs() (map[string]string, error) {
	fis, err := ctx.listServices()
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
	unitsAndJobs, err := ctx.deployedUnitsInitSystemJobs()
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
func (ctx *SimpleContext) service(unitName string, renderer shell.Renderer) (deployerService, error) {
	tag := names.NewUnitTag(unitName).String()
	svcName := "jujud-" + tag

	info := service.NewAgentInfo(
		service.AgentKindUnit,
		unitName,
		ctx.agentConfig.DataDir(),
		ctx.agentConfig.LogDir(),
	)

	// TODO(thumper): 2013-09-02 bug 1219630
	// As much as I'd like to remove JujuContainerType now, it is still
	// needed as MAAS still needs it at this stage, and we can't fix
	// everything at once.
	containerType := ctx.agentConfig.Value(agent.ContainerType)

	conf := service.ContainerAgentConf(info, renderer, containerType)
	return ctx.discoverService(svcName, conf)
}

func removeOnErr(err *error, path string) {
	if *err != nil {
		if err := os.RemoveAll(path); err != nil {
			logger.Warningf("installer: cannot remove %q: %v", path, err)
		}
	}
}
