// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"path"
	"sync"
	"time"

	"github.com/canonical/pebble/client"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/worker/uniter/container"
)

// PebbleClient describes the subset of github.com/canonical/pebble/client.Client that we
// need for the PebblePoller.
type PebbleClient interface {
	SysInfo() (*client.SysInfo, error)
	CloseIdleConnections()
}

// NewPebbleClientFunc is the function type used to create a PebbleClient.
type NewPebbleClientFunc func(*client.Config) (PebbleClient, error)

type pebblePoller struct {
	logger          Logger
	clock           clock.Clock
	tomb            tomb.Tomb
	newPebbleClient NewPebbleClientFunc

	containerNames    []string
	workloadEventChan chan string
	workloadEvents    container.WorkloadEvents

	mut           sync.Mutex
	pebbleBootIDs map[string]string
}

const (
	pebblePollInterval = 5 * time.Second
)

// NewPebblePoller starts a worker that polls the pebble interfaces
// of the supplied container list.
func NewPebblePoller(logger Logger,
	clock clock.Clock,
	containerNames []string,
	workloadEventChan chan string,
	workloadEvents container.WorkloadEvents,
	newPebbleClient NewPebbleClientFunc) worker.Worker {
	if newPebbleClient == nil {
		newPebbleClient = func(config *client.Config) (PebbleClient, error) {
			return client.New(config)
		}
	}
	p := &pebblePoller{
		logger:            logger,
		clock:             clock,
		workloadEventChan: workloadEventChan,
		workloadEvents:    workloadEvents,
		newPebbleClient:   newPebbleClient,
		pebbleBootIDs:     make(map[string]string),
	}
	for _, v := range containerNames {
		containerName := v
		p.tomb.Go(func() error {
			return p.run(containerName)
		})
	}
	return p
}

// Kill is part of the worker.Worker interface.
func (p *pebblePoller) Kill() {
	p.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (p *pebblePoller) Wait() error {
	return p.tomb.Wait()
}

func (p *pebblePoller) run(containerName string) error {
	timer := p.clock.NewTimer(pebblePollInterval)
	defer timer.Stop()
	for {
		select {
		case <-p.tomb.Dying():
			return tomb.ErrDying
		case <-timer.Chan():
			timer.Reset(pebblePollInterval)
			err := p.poll(containerName)
			if err != nil && err != tomb.ErrDying {
				p.logger.Errorf("pebble poll failed for container %q: %v", containerName, err)
			}
		}
	}
}

func (p *pebblePoller) poll(containerName string) error {
	config := &client.Config{
		Socket: path.Join("/charm/containers", containerName, "pebble.socket"),
	}
	pc, err := p.newPebbleClient(config)
	if err != nil {
		return errors.Annotate(err, "failed to create Pebble client")
	}
	defer pc.CloseIdleConnections()
	info, err := pc.SysInfo()
	if err != nil {
		return errors.Annotate(err, "failed to get pebble info")
	}

	p.mut.Lock()
	lastBootID, _ := p.pebbleBootIDs[containerName]
	p.mut.Unlock()
	if lastBootID == info.BootID {
		return nil
	}

	errChan := make(chan error, 1)
	eid := p.workloadEvents.AddWorkloadEvent(container.WorkloadEvent{
		Type:         container.ReadyEvent,
		WorkloadName: containerName,
	}, func(err error) {
		errChan <- errors.Trace(err)
	})
	defer p.workloadEvents.RemoveWorkloadEvent(eid)

	select {
	case p.workloadEventChan <- eid:
	case <-p.tomb.Dying():
		return tomb.ErrDying
	}

	select {
	case err := <-errChan:
		if err != nil {
			return errors.Trace(err)
		}
	case <-p.tomb.Dying():
		return tomb.ErrDying
	}

	p.mut.Lock()
	p.pebbleBootIDs[containerName] = info.BootID
	p.mut.Unlock()

	return nil
}
