package reboot

import (
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/factory"
	"github.com/juju/juju/instance"
)

var logger = loggo.GetLogger("juju.cmd.jujud.reboot")
var timeout = time.Duration(10 * time.Minute)
var rebootAfter = 15

func runCommand(args []string) error {
	_, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

var tmpFile = func() (*os.File, error) {
	f, err := ioutil.TempFile(os.TempDir(), "juju-reboot")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return f, nil
}

// Reboot implements the ExecuteReboot command which will reboot a machine
// once all containers have shut down, or a timeout is reached
type Reboot struct {
	acfg     agent.Config
	apistate api.Connection
	tag      names.MachineTag
	st       *reboot.State
}

func NewRebootWaiter(apistate api.Connection, acfg agent.Config) (*Reboot, error) {
	rebootState, err := apistate.Reboot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	tag, ok := acfg.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("Expected names.MachineTag, got: %T --> %v", acfg.Tag(), acfg.Tag())
	}
	return &Reboot{
		acfg:     acfg,
		st:       rebootState,
		tag:      tag,
		apistate: apistate,
	}, nil
}

// ExecuteReboot will wait for all running containers to stop, and then execute
// a shutdown or a reboot (based on the action param)
func (r *Reboot) ExecuteReboot(action params.RebootAction) error {
	err := r.waitForContainersOrTimeout()
	if err != nil {
		return errors.Trace(err)
	}

	err = scheduleAction(action, rebootAfter)
	if err != nil {
		return errors.Trace(err)
	}

	err = r.st.ClearReboot()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (r *Reboot) runningContainers() ([]instance.Instance, error) {
	runningInstances := []instance.Instance{}

	for _, val := range instance.ContainerTypes {
		managerConfig := container.ManagerConfig{container.ConfigName: container.DefaultNamespace}
		if namespace := r.acfg.Value(agent.Namespace); namespace != "" {
			managerConfig[container.ConfigName] = namespace
		}
		cfg := container.ManagerConfig(managerConfig)
		manager, err := factory.NewContainerManager(val, cfg, nil)
		if err != nil {
			logger.Warningf("Failed to get manager for container type %v: %v", val, err)
			continue
		}
		if !manager.IsInitialized() {
			continue
		}
		instances, err := manager.ListContainers()
		if err != nil {
			logger.Warningf("Failed to list containers: %v", err)
		}
		runningInstances = append(runningInstances, instances...)
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

		// Containers are still up after timeout. C'est la vie
		logger.Infof("Timeout reached waiting for containers to shutdown")
		quit <- true
	case err := <-c:
		return errors.Trace(err)

	}
	return nil
}
