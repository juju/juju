package converter

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.converter")

var _ worker.NotifyWatchHandler = (*UnitConverter)(nil)

type Environment struct {
	facade base.FacadeCaller
}

type UnitConverter struct {
	entity    *agent.Entity
	converter Converter
}

type Converter interface {
	WatchAPIHostPorts() (watcher.NotifyWatcher, error)
}

func NewUnitConverter(converter Converter, entity *agent.Entity) worker.Worker {
	return worker.NewNotifyWorker(&UnitConverter{
		converter: converter,
		entity:    entity,
	})
}

func (c *UnitConverter) checkForManageEnvironJob() bool {
	logger.Infof("checking if unit has been converted")
	for _, job := range c.entity.Jobs() {
		if job.NeedsState() {
			return true
		}
	}
	return false
}

func (c *UnitConverter) SetUp() (watcher.NotifyWatcher, error) {
	logger.Debugf("converter worker setup")
	if c.checkForManageEnvironJob() {
		return nil, errors.Errorf("already a state server, cannot convert")
	}
	return c.converter.WatchAPIHostPorts()
}

func (c *UnitConverter) Handle() error {
	if c.checkForManageEnvironJob() {
		logger.Debugf("we have been converted *sip kool-aid*")
		return worker.ErrTerminateAgent
	}
	return nil
}

func (r *UnitConverter) TearDown() error {
	// nothing to teardown.
	return nil
}
