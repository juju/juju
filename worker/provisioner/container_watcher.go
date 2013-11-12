// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/tomb"

	"launchpad.net/juju-core/instance"
	apiprovisioner "launchpad.net/juju-core/state/api/provisioner"
	"launchpad.net/juju-core/state/watcher"
)

type containerCallback func(ids []string) error

// ContainerWatcher invokes callback whenever a new container of the
// specified type is added to a machine.
type ContainerWatcher struct {
	ct       instance.ContainerType
	st       *apiprovisioner.State
	machine  *apiprovisioner.Machine
	tag      string
	tomb     tomb.Tomb
	callback containerCallback
}

// NewContainerWatcher returns a new Provisioner. When new machines
// are added to the state, it allocates instances from the environment
// and allocates them to the new machines.
func NewContainerWatcher(ct instance.ContainerType, st *apiprovisioner.State, tag string,
	callback containerCallback) *ContainerWatcher {

	cw := &ContainerWatcher{
		ct:       ct,
		st:       st,
		tag:      tag,
		callback: callback,
	}
	logger.Tracef("Starting %s container watcher for %q", cw.ct, cw.tag)
	go func() {
		defer cw.tomb.Done()
		cw.tomb.Kill(cw.loop())
	}()
	return cw
}

func (cw *ContainerWatcher) loop() error {
	machineWatcher, err := cw.getWatcher()
	if err != nil {
		return err
	}
	defer watcher.Stop(machineWatcher, &cw.tomb)

	for {
		select {
		case <-cw.tomb.Dying():
			logger.Infof("Shutting down container watcher %s", cw.tag)
			return tomb.ErrDying
		case ids, ok := <-machineWatcher.Changes():
			if !ok {
				return watcher.MustErr(machineWatcher)
			}
			if err := cw.callback(ids); err != nil {
				logger.Errorf("Container callback failed: %v", err)
				return err
			}
		}
	}
	return nil
}

func (cw *ContainerWatcher) getMachine() (*apiprovisioner.Machine, error) {
	if cw.machine == nil {
		var err error
		if cw.machine, err = cw.st.Machine(cw.tag); err != nil {
			logger.Errorf("%s is not in state", cw.tag)
			return nil, err
		}
	}
	return cw.machine, nil
}

func (cw *ContainerWatcher) getWatcher() (Watcher, error) {
	machine, err := cw.getMachine()
	if err != nil {
		return nil, err
	}
	return machine.WatchContainers(cw.ct)
}

// Err returns the reason why the ContainerWatcher has stopped or tomb.ErrStillAlive
// when it is still alive.
func (cw *ContainerWatcher) Err() (reason error) {
	return cw.tomb.Err()
}

// Kill implements worker.Worker.Kill.
func (cw *ContainerWatcher) Kill() {
	cw.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (cw *ContainerWatcher) Wait() error {
	return cw.tomb.Wait()
}

// Stop stops the ContainerWatcher and returns any error encountered while
// watching.
func (cw *ContainerWatcher) Stop() error {
	cw.tomb.Kill(nil)
	return cw.tomb.Wait()
}
