// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/container"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/rpc/params"
)

var logger = loggo.GetLogger("juju.cmd.jujud.reboot")
var timeout = 10 * time.Minute
var rebootAfter = 15

func runCommand(args []string) error {
	err := exec.Command(args[0], args[1:]...).Run()
	return errors.Trace(err)
}

var tmpFile = func() (*os.File, error) {
	f, err := os.CreateTemp(os.TempDir(), "juju-reboot")
	return f, errors.Trace(err)
}

// Reboot implements the ExecuteReboot command which will reboot a machine
// once all containers have shut down, or a timeout is reached
type Reboot struct {
	acfg   AgentConfig
	reboot RebootWaiter
}

func NewRebootWaiter(acfg agent.Config) (*Reboot, error) {
	// ensure we're only running on a machine agent.
	if _, ok := acfg.Tag().(names.MachineTag); !ok {
		return nil, errors.Errorf("Expected names.MachineTag, got: %T --> %v", acfg.Tag(), acfg.Tag())
	}
	return &Reboot{
		acfg:   &agentConfigShim{aCfg: acfg},
		reboot: rebootWaiterShim{},
	}, nil
}

// ExecuteReboot will wait for all running containers to stop, and then execute
// a shutdown or a reboot (based on the action param)
func (r *Reboot) ExecuteReboot(action params.RebootAction) error {
	if err := r.waitForContainersOrTimeout(); err != nil {
		return errors.Trace(err)
	}

	// Stop all units before issuing a reboot. During a reboot, the machine agent
	// will attempt to hold the execution lock until the reboot happens. However,
	// since the old file based locking method has been replaced with sockets, if
	// the machine agent is killed by the init system during shutdown, before the
	// unit agents, the lock is released and unit agents start running hooks.
	// When they in turn are killed, the hook is thrown into error state. If
	// automatic retries are disabled, the hook remains in error state.
	if err := r.stopDeployedUnits(); err != nil {
		return errors.Trace(err)
	}

	if err := r.reboot.ScheduleAction(action, rebootAfter); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (r *Reboot) stopDeployedUnits() error {
	services, err := r.reboot.ListServices()
	if err != nil {
		return err
	}
	for _, svcName := range services {
		if strings.HasPrefix(svcName, `jujud-unit-`) {
			svc, err := r.reboot.NewServiceReference(svcName)
			if err != nil {
				return err
			}
			logger.Debugf("Stopping unit agent: %q", svcName)
			if err = svc.Stop(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reboot) runningContainers() ([]instances.Instance, error) {
	var runningInstances []instances.Instance
	modelUUID := r.acfg.Model().Id()
	for _, val := range instance.ContainerTypes {
		managerConfig := container.ManagerConfig{
			container.ConfigModelUUID: modelUUID,
		}
		cfg := managerConfig
		manager, err := r.reboot.NewContainerManager(val, cfg)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to get manager for container type %v", val)
		}
		if !manager.IsInitialized() {
			logger.Infof("container type %q not supported", val)
			continue
		}
		containers, err := manager.ListContainers()
		if err != nil {
			return nil, errors.Annotate(err, "failed to list containers")
		}
		runningInstances = append(runningInstances, containers...)
	}
	return runningInstances, nil
}

func (r *Reboot) waitForContainersOrTimeout() error {
	c := make(chan error, 1)
	quit := make(chan bool, 1)
	go func() {
		for {
			select {
			case <-quit:
				c <- nil
				return
			default:
				containers, err := r.runningContainers()
				if err != nil {
					c <- err
					return
				}
				if len(containers) == 0 {
					c <- nil
					return
				}
				logger.Warningf("Waiting for containers to shutdown: %v", containers)
				time.Sleep(1 * time.Second)
			}
		}
	}()

	select {
	case <-time.After(timeout):
		// TODO(fwereade): 2016-03-17 lp:1558657
		// Containers are still up after timeout. C'est la vie
		quit <- true
		return errors.New("Timeout reached waiting for containers to shutdown")
	case err := <-c:
		return errors.Trace(err)
	}
}
