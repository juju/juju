// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service"
	"github.com/juju/juju/version"
)

// APICalls defines the interface to the API that the simple context needs.
type APICalls interface {
	ConnectionInfo() (params.DeployerConnectionValues, error)
}

type services interface {
	ListEnabled() ([]string, error)
	IsEnabled(name string) (bool, error)
	Install(name string, conf service.Conf) error
	NewAgentService(tag names.Tag, paths service.AgentPaths, env map[string]string) (*service.Service, error)
}

// SimpleContext is a Context that manages unit deployments on the local system.
type SimpleContext struct {

	// api is used to get the current state server addresses at the time the
	// given unit is deployed.
	api APICalls

	// agentConfig returns the agent config for the machine agent that is
	// running the deployer.
	agentConfig agent.Config

	// services is the wrapper around the host's init system.
	services services
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
func NewSimpleContext(agentConfig agent.Config, api APICalls) (*SimpleContext, error) {
	services, err := service.DiscoverServices(agentConfig.DataDir())
	if err != nil {
		return nil, errors.Trace(err)
	}

	ctx := &SimpleContext{
		api:         api,
		agentConfig: agentConfig,
		services:    services,
	}
	return ctx, nil
}

func (ctx *SimpleContext) AgentConfig() agent.Config {
	return ctx.agentConfig
}

func (ctx *SimpleContext) DeployUnit(unitName, initialPassword string) (err error) {
	tag := names.NewUnitTag(unitName)

	svc, err := ctx.service(tag)
	if err != nil {
		return errors.Trace(err)
	}

	// Fail if the unit is already deployed.
	deployed, err := ctx.isDeployed(svc.Name())
	if err != nil {
		return errors.Trace(err)
	}
	if deployed {
		return fmt.Errorf("unit %q is already deployed", unitName)
	}

	// Link the current tools for use by the new agent.
	toolsDir, err := ctx.linkTools(tag)
	if err != nil {
		return errors.Trace(err)
	}
	defer removeOnErr(&err, toolsDir)

	// Update the agent config and write it out.
	agentConf, err := ctx.newAgentConf(tag, initialPassword)
	if err != nil {
		return errors.Trace(err)
	}
	if err := agentConf.Write(); err != nil {
		return err
	}
	defer removeOnErr(&err, agentConf.Dir())

	// Install the service into the init system.
	svcConf := svc.Conf()
	err = ctx.services.Install(svc.Name(), svcConf)
	return errors.Trace(err)
}

func (ctx SimpleContext) isDeployed(svcName string) (bool, error) {
	enabled, err := ctx.services.IsEnabled(svcName)
	return enabled, errors.Trace(err)
}

func (ctx SimpleContext) linkTools(tag names.Tag) (string, error) {
	dataDir := ctx.agentConfig.DataDir()
	toolsDir := tools.ToolsDir(dataDir, tag.String())

	// TODO(dfc)
	_, err := tools.ChangeAgentTools(dataDir, tag.String(), version.Current)
	// TODO(dfc)
	if err != nil {
		return "", errors.Trace(err)
	}
	return toolsDir, nil
}

func (ctx SimpleContext) newAgentConf(tag names.Tag, initialPassword string) (agent.ConfigSetterWriter, error) {
	dataDir := ctx.agentConfig.DataDir()
	logDir := ctx.agentConfig.LogDir()

	result, err := ctx.api.ConnectionInfo()
	if err != nil {
		return nil, errors.Trace(err)
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
	return conf, errors.Trace(err)
}

func (ctx *SimpleContext) RecallUnit(unitName string) error {
	tag := names.NewUnitTag(unitName)

	svc, err := ctx.service(tag)
	if err != nil {
		return errors.Trace(err)
	}

	// Fail if not deployed.
	enabled, err := svc.IsEnabled()
	if err != nil {
		return errors.Trace(err)
	}
	if !enabled {
		return errors.Errorf("unit %q is not deployed", unitName)
	}

	// Uninstall the service.
	if err := svc.Remove(); err != nil {
		return errors.Trace(err)
	}

	// Clean up files.
	dataDir := ctx.agentConfig.DataDir()
	agentDir := agent.Dir(dataDir, tag)
	// Recursivley change mode to 777 on windows to avoid
	// Operation not permitted errors when deleting the agentDir
	if err := recursiveChmod(agentDir, os.FileMode(0777)); err != nil {
		return errors.Trace(err)
	}
	if err := os.RemoveAll(agentDir); err != nil {
		return errors.Trace(err)
	}
	// TODO(dfc) should take a Tag
	toolsDir := tools.ToolsDir(dataDir, tag.String())
	err = os.Remove(toolsDir)
	return errors.Trace(err)
}

func (ctx *SimpleContext) DeployedUnits() ([]string, error) {
	tags, err := service.ListAgents(ctx.services)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var names []string
	for _, tag := range tags {
		if tag.Kind() != "unit" {
			continue
		}
		names = append(names, tag.Id())
	}
	return names, nil
}

// service returns a service.Service corresponding to the specified
// unit.
func (ctx *SimpleContext) service(tag names.Tag) (*service.Service, error) {
	// TODO(thumper): 2013-09-02 bug 1219630
	// As much as I'd like to remove JujuContainerType now, it is still
	// needed as MAAS still needs it at this stage, and we can't fix
	// everything at once.
	containerType := ctx.agentConfig.Value(agent.ContainerType)
	envVars := map[string]string{
		osenv.JujuContainerTypeEnvKey: containerType,
	}
	osenv.MergeEnvironment(envVars, osenv.FeatureFlags())

	svc, err := ctx.services.NewAgentService(tag, ctx.agentConfig, envVars)
	return svc, errors.Trace(err)
}

func removeOnErr(err *error, path string) {
	if *err != nil {
		if err := os.RemoveAll(path); err != nil {
			logger.Warningf("installer: cannot remove %q: %v", path, err)
		}
	}
}
