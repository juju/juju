package container

import (
	"os/exec"
	"runtime"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/worker"
)

var RunningInContainer = func() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	/* running-in-container is in init-scripts-helpers, and is smart enough
	 * to ask both systemd and upstart whether or not they know if the task
	 * is running in a container.
	 */
	cmd := exec.Command("running-in-container")
	return cmd.Run() == nil
}

func ContainersSupported() bool {
	return !RunningInContainer()
}

type ContainerAgentConfig interface {
	Tag() names.Tag
	Value(string) string
}

type RunningContainersFn func() ([]instance.Instance, error)

func WaitForContainerTeardownWorker(
	clock clock.Clock,
	newContainerManager NewContainerManagerFn,
	agentConfig ContainerAgentConfig,
) (worker.Worker, error) {
	workerCallFn, err := WaitForContainerTeardownWorkerCallFn(clock, newContainerManager, agentConfig)
	if err != nil {
		return nil, err
	}
	return worker.NewPeriodicWorker(
		workerCallFn,
		1*time.Second,
		worker.NewTimer,
	), nil
}

func WaitForContainerTeardownWorkerCallFn(
	clock clock.Clock,
	newContainerManager NewContainerManagerFn,
	agentConfig ContainerAgentConfig,
) (worker.PeriodicWorkerCall, error) {
	if err := validateAgentConfig(agentConfig); err != nil {
		return nil, errors.Trace(err)
	}
	runningContainers := func() ([]instance.Instance, error) {
		return runningContainers(agentConfig, newContainerManager)
	}

	return func(<-chan struct{}) error {
		containers, err := runningContainers()
		if len(containers) == 0 {
			return worker.ErrKilled
		}
		logger.Infof("Waiting for containers to shutdown: %v", containers)
		return err
	}, nil
}

func validateAgentConfig(agentConfig ContainerAgentConfig) error {
	if _, ok := agentConfig.Tag().(names.MachineTag); !ok {
		return errors.Errorf("expected names.MachineTag, got: %T --> %v", agentConfig.Tag(), agentConfig.Tag())
	}
	return nil
}

type NewContainerManagerFn func(instance.ContainerType, ManagerConfig) (ContainerManager, error)

type ContainerManager interface {
	IsInitialized() bool
	ListContainers() ([]instance.Instance, error)
}

func runningContainers(agentConfig ContainerAgentConfig, newContainerManager NewContainerManagerFn) ([]instance.Instance, error) {
	var runningInstances []instance.Instance

	for _, val := range instance.ContainerTypes {
		managerConfig := ManagerConfig{ConfigName: DefaultNamespace}
		if namespace := agentConfig.Value(agent.Namespace); namespace != "" {
			managerConfig[ConfigName] = namespace
		}
		cfg := ManagerConfig(managerConfig)
		manager, err := newContainerManager(val, cfg)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to get manager for container type %v", val)
		}
		if !manager.IsInitialized() {
			logger.Infof("container type %q not supported", val)
			continue
		}
		instances, err := manager.ListContainers()
		if err != nil {
			return nil, errors.Annotate(err, "failed to list containers")
		}
		runningInstances = append(runningInstances, instances...)
	}

	return runningInstances, nil
}
