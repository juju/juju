// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcache

import (
	"sync"

	"github.com/juju/errors"
	"github.com/kr/pretty"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
)

// BackingWatcher describes watcher methods that supply deltas from state to
// this worker. In-theatre it is satisfied by a state.Multiwatcher.
type BackingWatcher interface {
	Next() ([]multiwatcher.Delta, error)
	Stop() error
}

// Config describes the necessary fields for NewWorker.
type Config struct {
	Logger               Logger
	PrometheusRegisterer prometheus.Registerer
	Cleanup              func()

	// Notify is used primarily for testing, and is passed through
	// to the cache.Controller. It is called every time the controller
	// processes an event.
	Notify func(interface{})

	// WatcherFactory supplies the watcher that supplies deltas from state.
	// We use a factory because we do not allow the worker loop to be crashed
	// by a watcher that stops in an error state.
	// Watcher acquisition my occur multiple times during a worker life-cycle.
	WatcherFactory func() BackingWatcher
}

// Validate ensures all the necessary values are specified
func (c *Config) Validate() error {
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	if c.WatcherFactory == nil {
		return errors.NotValidf("missing watcher factory")
	}
	if c.PrometheusRegisterer == nil {
		return errors.NotValidf("missing prometheus registerer")
	}
	if c.Cleanup == nil {
		return errors.NotValidf("missing cleanup func")
	}
	return nil
}

type cacheWorker struct {
	config     Config
	catacomb   catacomb.Catacomb
	controller *cache.Controller
	changes    chan interface{}
	watcher    BackingWatcher
	mu         sync.Mutex
}

// NewWorker creates a new cacheWorker, and starts an
// all model watcher.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &cacheWorker{
		config:  config,
		changes: make(chan interface{}),
	}
	controller, err := cache.NewController(
		cache.ControllerConfig{
			Changes: w.changes,
			Notify:  config.Notify,
		})
	if err != nil {
		return nil, errors.Trace(err)
	}
	w.controller = controller
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.controller},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Report returns information that is used in the dependency engine report.
func (c *cacheWorker) Report() map[string]interface{} {
	return c.controller.Report()
}

func (c *cacheWorker) loop() error {
	defer c.config.Cleanup()

	allWatcherStarts := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "juju_worker_modelcache",
		Name:      "watcher_starts",
		Help:      "The number of times the all model watcher has been started.",
	})

	collector := cache.NewMetricsCollector(c.controller)
	_ = c.config.PrometheusRegisterer.Register(collector)
	_ = c.config.PrometheusRegisterer.Register(allWatcherStarts)
	defer c.config.PrometheusRegisterer.Unregister(allWatcherStarts)
	defer c.config.PrometheusRegisterer.Unregister(collector)

	watcherChanges := make(chan []multiwatcher.Delta)
	// This worker needs to be robust with respect to the multiwatcher errors.
	// If we get an unexpected error we should get a new allWatcher.
	// We don't want a weird error in the multiwatcher taking down the apiserver,
	// which is what would happen if this worker errors out.
	// The cached controller takes care of invalidation
	// via its own mark/sweep logic.
	var wg sync.WaitGroup
	wg.Add(1)
	defer func() {
		c.mu.Lock()
		// If we have been stopped before we have properly been started
		// there may not be a watcher yet.
		if c.watcher != nil {
			_ = c.watcher.Stop()
		}
		c.mu.Unlock()
		wg.Wait()
	}()

	go func() {
		// Ensure we don't leave the main loop until the goroutine is done.
		defer wg.Done()
		for {
			c.mu.Lock()
			select {
			case <-c.catacomb.Dying():
				c.mu.Unlock()
				return
			default:
				// Continue through.
			}

			// Each time the watcher is restarted,
			// mark the cache residents as stale.
			c.controller.Mark()

			allWatcherStarts.Inc()
			c.watcher = c.config.WatcherFactory()
			c.mu.Unlock()

			err := c.processWatcher(watcherChanges)
			if err == nil {
				// We are done, so exit
				_ = c.watcher.Stop()
				return
			}
			c.config.Logger.Errorf("watcher error, %v, getting new watcher", err)
			_ = c.watcher.Stop()
		}
	}()

	for {
		select {
		case <-c.catacomb.Dying():
			return c.catacomb.ErrDying()
		case deltas := <-watcherChanges:
			// Process changes and send info down changes channel
			for _, d := range deltas {
				if logger := c.config.Logger; logger.IsTraceEnabled() {
					logger.Tracef(pretty.Sprint(d))
				}
				value := c.translate(d)
				if value != nil {
					select {
					case c.changes <- value:
					case <-c.catacomb.Dying():
						return nil
					}
				}
			}

			// Evict any stale residents.
			c.controller.Sweep()
		}
	}
}

func (c *cacheWorker) processWatcher(watcherChanges chan<- []multiwatcher.Delta) error {
	for {
		deltas, err := c.watcher.Next()
		if err != nil {
			if errors.Cause(err) == state.ErrStopped {
				return nil
			} else {
				return errors.Trace(err)
			}
		}
		select {
		case <-c.catacomb.Dying():
			return nil
		case watcherChanges <- deltas:
		}
	}
}

func (c *cacheWorker) translate(d multiwatcher.Delta) interface{} {
	id := d.Entity.EntityId()
	switch id.Kind {
	case "model":
		if d.Removed {
			return cache.RemoveModel{
				ModelUUID: id.ModelUUID,
			}
		}
		value, ok := d.Entity.(*multiwatcher.ModelInfo)
		if !ok {
			c.config.Logger.Errorf("unexpected type %T", d.Entity)
			return nil
		}
		return cache.ModelChange{
			ModelUUID: value.ModelUUID,
			Name:      value.Name,
			Life:      life.Value(value.Life),
			Owner:     value.Owner,
			Config:    value.Config,
			Status:    coreStatus(value.Status),
			// TODO: constraints, sla
		}
	case "application":
		if d.Removed {
			return cache.RemoveApplication{
				ModelUUID: id.ModelUUID,
				Name:      id.Id,
			}
		}
		value, ok := d.Entity.(*multiwatcher.ApplicationInfo)
		if !ok {
			c.config.Logger.Errorf("unexpected type %T", d.Entity)
			return nil
		}
		return cache.ApplicationChange{
			ModelUUID:       value.ModelUUID,
			Name:            value.Name,
			Exposed:         value.Exposed,
			CharmURL:        value.CharmURL,
			Life:            life.Value(value.Life),
			MinUnits:        value.MinUnits,
			Constraints:     value.Constraints,
			Config:          value.Config,
			Subordinate:     value.Subordinate,
			Status:          coreStatus(value.Status),
			WorkloadVersion: value.WorkloadVersion,
		}
	case "machine":
		if d.Removed {
			return cache.RemoveMachine{
				ModelUUID: id.ModelUUID,
				Id:        id.Id,
			}
		}
		value, ok := d.Entity.(*multiwatcher.MachineInfo)
		if !ok {
			c.config.Logger.Errorf("unexpected type %T", d.Entity)
			return nil
		}
		return cache.MachineChange{
			ModelUUID:                value.ModelUUID,
			Id:                       value.Id,
			InstanceId:               value.InstanceId,
			AgentStatus:              coreStatus(value.AgentStatus),
			Life:                     life.Value(value.Life),
			Config:                   value.Config,
			Series:                   value.Series,
			SupportedContainers:      value.SupportedContainers,
			SupportedContainersKnown: value.SupportedContainersKnown,
			HardwareCharacteristics:  value.HardwareCharacteristics,
			CharmProfiles:            value.CharmProfiles,
			Addresses:                networkAddresses(value.Addresses),
			HasVote:                  value.HasVote,
			WantsVote:                value.WantsVote,
		}
	case "unit":
		if d.Removed {
			return cache.RemoveUnit{
				ModelUUID: id.ModelUUID,
				Name:      id.Id,
			}
		}
		value, ok := d.Entity.(*multiwatcher.UnitInfo)
		if !ok {
			c.config.Logger.Errorf("unexpected type %T", d.Entity)
			return nil
		}
		return cache.UnitChange{
			ModelUUID:      value.ModelUUID,
			Name:           value.Name,
			Application:    value.Application,
			Series:         value.Series,
			CharmURL:       value.CharmURL,
			Life:           life.Value(value.Life),
			PublicAddress:  value.PublicAddress,
			PrivateAddress: value.PrivateAddress,
			MachineId:      value.MachineId,
			Ports:          networkPorts(value.Ports),
			PortRanges:     networkPortRanges(value.PortRanges),
			Principal:      value.Principal,
			Subordinate:    value.Subordinate,
			WorkloadStatus: coreStatus(value.WorkloadStatus),
			AgentStatus:    coreStatus(value.AgentStatus),
		}
	case "charm":
		if d.Removed {
			return cache.RemoveCharm{
				ModelUUID: id.ModelUUID,
				CharmURL:  id.Id,
			}
		}
		value, ok := d.Entity.(*multiwatcher.CharmInfo)
		if !ok {
			c.config.Logger.Errorf("unexpected type %T", d.Entity)
			return nil
		}
		return cache.CharmChange{
			ModelUUID:  value.ModelUUID,
			CharmURL:   value.CharmURL,
			LXDProfile: coreLXDProfile(value.LXDProfile),
		}
	default:
		return nil
	}
}

// Kill is part of the worker.Worker interface.
func (c *cacheWorker) Kill() {
	c.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (c *cacheWorker) Wait() error {
	return c.catacomb.Wait()
}

func coreStatus(info multiwatcher.StatusInfo) status.StatusInfo {
	return status.StatusInfo{
		Status:  info.Current,
		Message: info.Message,
		Data:    info.Data,
		Since:   info.Since,
	}
}

func networkPorts(delta []multiwatcher.Port) []network.Port {
	ports := make([]network.Port, len(delta))
	for i, d := range delta {
		ports[i] = network.Port{
			Protocol: d.Protocol,
			Number:   d.Number,
		}
	}
	return ports
}

func networkPortRanges(delta []multiwatcher.PortRange) []network.PortRange {
	ports := make([]network.PortRange, len(delta))
	for i, d := range delta {
		ports[i] = network.PortRange{
			Protocol: d.Protocol,
			FromPort: d.FromPort,
			ToPort:   d.ToPort,
		}
	}
	return ports
}

func networkAddresses(delta []multiwatcher.Address) []network.Address {
	addresses := make([]network.Address, len(delta))
	for i, d := range delta {
		addresses[i] = network.Address{
			Value:           d.Value,
			Type:            network.AddressType(d.Type),
			Scope:           network.Scope(d.Scope),
			SpaceName:       network.SpaceName(d.SpaceName),
			SpaceProviderId: network.Id(d.SpaceProviderId),
		}
	}
	return addresses
}

func coreLXDProfile(delta *multiwatcher.Profile) lxdprofile.Profile {
	if delta == nil {
		return lxdprofile.Profile{}
	}
	return lxdprofile.Profile{
		Config:      delta.Config,
		Description: delta.Description,
		Devices:     delta.Devices,
	}
}
