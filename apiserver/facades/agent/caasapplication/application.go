// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	"github.com/juju/utils/v2"

	"github.com/juju/juju/agent"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.caasapplication")

type Facade struct {
	auth      facade.Authorizer
	resources facade.Resources
	state     State
	model     Model
	clock     clock.Clock
}

// NewStateFacade provides the signature required for facade registration.
func NewStateFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	return NewFacade(resources, authorizer, &stateShim{ctx.State()}, ctx.StatePool().Clock())
}

// NewFacade returns a new CAASOperator facade.
func NewFacade(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st State,
	clock clock.Clock,
) (*Facade, error) {
	if !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Facade{
		auth:      authorizer,
		resources: resources,
		state:     st,
		model:     model,
		clock:     clock,
	}, nil
}

// UnitIntroduction sets the status of each given entity.
func (f *Facade) UnitIntroduction(args params.CAASUnitIntroductionArgs) (params.CAASUnitIntroductionResult, error) {
	tag, ok := f.auth.GetAuthTag().(names.ApplicationTag)
	if !ok {
		return params.CAASUnitIntroductionResult{}, apiservererrors.ErrPerm
	}

	errResp := func(err error) (params.CAASUnitIntroductionResult, error) {
		return params.CAASUnitIntroductionResult{Error: apiservererrors.ServerError(err)}, nil
	}

	if args.PodName == "" {
		return errResp(errors.NotValidf("pod-name"))
	}
	if args.PodUUID == "" {
		return errResp(errors.NotValidf("pod-uuid"))
	}

	logger.Debugf("introducing pod %q (%q)", args.PodName, args.PodUUID)

	application, err := f.state.Application(tag.Name)
	if err != nil {
		return errResp(err)
	}

	if application.Life() != state.Alive {
		return errResp(errors.NotProvisionedf("application"))
	}

	// TODO(embedded): handle deployment other than statefulset
	// ch, _, err := application.Charm()
	// if err != nil {
	// 	return errResp(err)
	// }
	deploymentType := caas.DeploymentStateful

	containerID := args.PodName
	var unitName *string
	switch deploymentType {
	case caas.DeploymentStateful:
		splitPodName := strings.Split(args.PodName, "-")
		ord, err := strconv.Atoi(splitPodName[len(splitPodName)-1])
		if err != nil {
			return errResp(err)
		}
		n := fmt.Sprintf("%s/%d", application.Name(), ord)
		unitName = &n
	case caas.DeploymentStateless, caas.DeploymentDaemon:
		// Both handled the same way.
	default:
		return errResp(errors.NotSupportedf("unknown deployment type"))
	}

	var unit Unit = nil
	if unitName != nil {
		unit, err = f.state.Unit(*unitName)
		if err != nil && !errors.IsNotFound(err) {
			return errResp(err)
		}
	} else {
		containers, err := f.model.Containers(containerID)
		if err != nil {
			return errResp(err)
		}

		if len(containers) != 0 {
			container := containers[0]
			unit, err = f.state.Unit(container.Unit())
			if err != nil {
				return errResp(err)
			}
			logger.Debugf("pod %q matched unit %q", args.PodName, unit.Tag().String())
		}

		// Find an existing unit that isn't yet assigned.
		if unit == nil {
			units, err := application.AllUnits()
			if err != nil {
				return errResp(err)
			}
			for _, existingUnit := range units {
				info, err := existingUnit.ContainerInfo()
				if errors.IsNotFound(err) {
					unit = existingUnit
					break
				} else if err != nil {
					return errResp(err)
				}
				if info.ProviderId() == "" {
					unit = existingUnit
					break
				}
			}
			if unit != nil {
				logger.Debugf("pod %q matched unused unit %q", args.PodName, unit.Tag().String())
			}
		}
	}

	if unit != nil && unit.Life() != state.Alive {
		retryErr := errors.New("retry")
		call := retry.CallArgs{
			Clock:    f.clock,
			Delay:    5 * time.Second,
			Attempts: 12,
			IsFatalError: func(err error) bool {
				return err != retryErr
			},
			Func: func() error {
				err := unit.Refresh()
				if errors.IsNotFound(err) {
					unit = nil
					return nil
				} else if err != nil {
					return retryErr
				}
				switch unit.Life() {
				case state.Alive:
					return nil
				case state.Dying, state.Dead:
					logger.Debugf("still waiting for old unit %q to cleanup", unit.Tag().String())
					return retryErr
				default:
					return errors.Errorf("unknown life state")
				}
			},
		}
		err := retry.Call(call)
		if err != nil {
			return errResp(errors.Annotatef(err,
				"failed waiting for old unit %q to cleanup", unit.Tag().String()))
		}
	}

	// Force update of provider-id.
	if unit != nil {
		update := unit.UpdateOperation(state.UnitUpdateProperties{
			ProviderId: &containerID,
		})
		var unitUpdate state.UpdateUnitsOperation
		unitUpdate.Updates = append(unitUpdate.Updates, update)
		err = application.UpdateUnits(&unitUpdate)
		if err != nil {
			return errResp(err)
		}
		logger.Debugf("unit %q updated provider id to %q", unit.Tag().String(), containerID)
	}

	// Create a new unit if we never found one.
	if unit == nil {
		unit, err = application.AddUnit(state.AddUnitParams{
			UnitName:   unitName,
			ProviderId: &containerID,
		})
		if err != nil {
			return errResp(err)
		}
		logger.Debugf("created new unit %q for pod %q", unit.Tag().String(), args.PodName)
	}

	password, err := utils.RandomPassword()
	if err != nil {
		return errResp(err)
	}

	err = unit.SetPassword(password)
	if err != nil {
		return errResp(err)
	}

	controllerConfig, err := f.state.ControllerConfig()
	if err != nil {
		return errResp(err)
	}

	apiHostPorts, err := f.state.APIHostPortsForAgents()
	if err != nil {
		return errResp(err)
	}

	addrs := []string(nil)
	for _, hostPorts := range apiHostPorts {
		ordered := hostPorts.HostPorts().PrioritizedForScope(network.ScopeMatchCloudLocal)
		for _, addr := range ordered {
			if addr != "" {
				addrs = append(addrs, addr)
			}
		}
	}

	caCert, _ := controllerConfig.CACert()
	version, _ := f.model.AgentVersion()
	dataDir, _ := paths.DataDir("kubernetes")
	logDir, _ := paths.LogDir("kubernetes")

	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir: dataDir,
				LogDir:  logDir,
			},
			Tag:               unit.Tag(),
			Controller:        f.model.ControllerTag(),
			Model:             f.model.Tag().(names.ModelTag),
			APIAddresses:      addrs,
			CACert:            caCert,
			Password:          password,
			UpgradedToVersion: version,
		},
	)
	if err != nil {
		return errResp(errors.Annotatef(err, "creating new agent config"))
	}

	agentConfBytes, err := conf.Render()
	if err != nil {
		return errResp(err)
	}

	res := params.CAASUnitIntroductionResult{
		Result: &params.CAASUnitIntroduction{
			UnitName:  unit.Tag().Id(),
			AgentConf: agentConfBytes,
		},
	}
	return res, nil
}
