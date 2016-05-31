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
	err := exec.Command(args[0], args[1:]...).Run()
	return errors.Trace(err)
}

var tmpFile = func() (*os.File, error) {
	f, err := ioutil.TempFile(os.TempDir(), "juju-reboot")
	return f, errors.Trace(err)
}

// Reboot implements the ExecuteReboot command which will reboot a machine
// once all containers have shut down, or a timeout is reached
type Reboot struct {
	acfg     agent.Config
	apistate api.Connection
	tag      names.MachineTag
	st       reboot.State
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
	if err := r.waitForContainersOrTimeout(); err != nil {
		return errors.Trace(err)
	}

	if err := scheduleAction(action, rebootAfter); err != nil {
		return errors.Trace(err)
	}

	err := r.st.ClearReboot()
	return errors.Trace(err)
}

func (r *Reboot) runningContainers() ([]instance.Instance, error) {
	var runningInstances []instance.Instance
	modelUUID := r.acfg.Model().Id()
	for _, val := range instance.ContainerTypes {
		managerConfig := container.ManagerConfig{
			container.ConfigModelUUID: modelUUID}
		cfg := container.ManagerConfig(managerConfig)
		manager, err := factory.NewContainerManager(val, cfg, nil)
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
