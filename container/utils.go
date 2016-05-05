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

func ContainerTeardownOrTimeout(clock clock.Clock, teardown <-chan error, timeout time.Duration) <-chan error {
	teardownOrTimeout := make(chan error)
	go func() {
		select {
		case err := <-teardown:
			teardownOrTimeout <- err
		case <-clock.After(timeout):
			teardownOrTimeout <- errors.New("timeout reached waiting for containers to shutdown")
		}
	}()
	return teardownOrTimeout
}

type ContainerAgentConfig interface {
	Tag() names.Tag
	Value(string) string
}

func ContainerTeardown(
	clock clock.Clock,
	newContainerManager NewContainerManagerFn,
	agentConfig ContainerAgentConfig,
	quit <-chan struct{},
) (<-chan error, error) {
	if err := validateAgentConfig(agentConfig); err != nil {
		return nil, errors.Trace(err)
	}
	runningContainers := func() ([]instance.Instance, error) {
		return runningContainers(agentConfig, newContainerManager)
	}

	teardownComplete := make(chan error)
	go func() {
		defer close(teardownComplete)
		for {
			containers, err := runningContainers()
			if err != nil || len(containers) == 0 {
				teardownComplete <- err
				return
			}
			logger.Infof("Waiting for containers to shutdown: %v", containers)

			select {
			case <-quit:
				logger.Infof("No longer waiting for containers to shutdown")
				// Rather than sending a signal on the channel, allow
				// the defer to close it to indicate that we quit, we
				// didn't successfully tear down.
				return
			case <-clock.After(1 * time.Second):
			}
		}
	}()

	return teardownComplete, nil
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
