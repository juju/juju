// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

var serviceDiscovery = service.DiscoverService

// stateStepsFor29 returns upgrade steps for Juju 2.9.0
func stateStepsFor29() []Step {
	return []Step{
		&upgradeStep{
			description: "add charm-hub-url to model config",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddCharmHubToModelConfig()
			},
		},
		&upgradeStep{
			description: "roll up and convert opened port documents into the new endpoint-aware format",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RollUpAndConvertOpenedPortDocuments()
			},
		},
		&upgradeStep{
			description: "add charm-origin to applications",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddCharmOriginToApplications()
			},
		},
		&upgradeStep{
			description: "add Azure provider network config",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddAzureProviderNetworkConfig()
			},
		},
	}
}

// stepsFor29 returns upgrade steps for Juju 2.9.0.
func stepsFor29() []Step {
	return []Step{
		&upgradeStep{
			description: "store deployed units in machine agent.conf",
			targets:     []Target{HostMachine},
			run:         storeDeployedUnitsInMachineAgentConf,
		},
	}
}

func storeDeployedUnitsInMachineAgentConf(ctx Context) error {
	// Lookup the names of all unit agents installed on this machine.
	agentConf := ctx.AgentConfig()
	ctxTag := agentConf.Tag()
	if ctxTag.Kind() != names.MachineTagKind {
		logger.Infof("skipping agent %q, not a machine", ctxTag.String())
		return nil
	}
	_, unitNames, _, err := service.FindAgents(agentConf.DataDir())
	if err != nil {
		return errors.Annotate(err, "looking up unit agents")
	}
	if len(unitNames) == 0 {
		// No units, nothing to do.
		return nil
	}

	sort.Strings(unitNames)
	var deployed []string
	for _, tagStr := range unitNames {
		// We know that these are all valid unit tags, but handle the
		// error anyway.
		tag, err := names.ParseUnitTag(tagStr)
		if err != nil {
			// Ignore it and continue, but it shouldn't happen
			continue
		}
		logger.Debugf("processing unit %q", tag.Id())
		deployed = append(deployed, tag.Id())

		// Remove the services for each unit.
		serviceName := "jujud-" + tagStr
		svc, err := serviceDiscovery(serviceName, common.Conf{})
		if err != nil {
			// We shouldn't get an error here as we should understand the underlying
			// service init system used by the operating system, if we do get an error,
			// log it and continue.
			logger.Errorf("service discovery: %v", err)
			continue
		}
		installed, err := svc.Installed()
		if err != nil {
			// We shouldn't get an error here as the name should be valid.
			// NOTE: check service name length etc...
			logger.Errorf("service installed: %v", err)
			continue
		}
		if installed {
			logger.Debugf("stopping service for unit %q", tag.Id())
			if err := svc.Stop(); err != nil {
				// Log a stopped error, but continue to try to remove.
				logger.Errorf("stopping service for unit %q: %v", tagStr, err)
			}
			logger.Debugf("removing service for unit %q", tag.Id())
			if err := svc.Remove(); err != nil {
				return errors.Trace(err)
			}
		} else {
			logger.Debugf("service not installed for unit %q", tag.Id())
		}
	}

	agentConf.SetValue("deployed-units", strings.Join(deployed, ","))
	return nil
}
