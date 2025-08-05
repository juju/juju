// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/utils/scriptrunner"
)

var logger = loggo.GetLogger("juju.worker.fanconfigurer")

type FanConfigurer struct {
	catacomb catacomb.Catacomb
	config   FanConfigurerConfig
	clock    clock.Clock
	mu       sync.Mutex
}

type FanConfigurerFacade interface {
	FanConfig(tag names.MachineTag) (network.FanConfig, error)
	WatchForFanConfigChanges() (watcher.NotifyWatcher, error)
}

type FanConfigurerConfig struct {
	Facade FanConfigurerFacade
	Tag    names.MachineTag
}

// processNewConfig acts on a new fan config.
func (fc *FanConfigurer) processNewConfig() error {
	logger.Debugf("Processing new fan config")
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// In 4.0 fan-config, along with fanconfigurer facade has been removed.
	// If we've been migrated from 3.6 to 4.0 the agent will attempt to
	// use the new facade, but it will fail.
	// Part of the migration steps to 4.0 prevents a migration with fan-config
	// set, so the code below is dead code.
	fanConfig, err := fc.config.Facade.FanConfig(fc.config.Tag)
	if errors.Is(err, errors.NotImplemented) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	if len(fanConfig) == 0 {
		logger.Debugf("Fan not enabled")
		// TODO(wpk) 2017-08-05 We have to clean this up!
		return nil
	}

	for i, fan := range fanConfig {
		logger.Debugf("Adding config for %d: %s %s", i, fan.Underlay, fan.Overlay)
		line := fmt.Sprintf("fanatic enable-fan -u %s -o %s", fan.Underlay, fan.Overlay)
		result, err := scriptrunner.RunCommand(line, os.Environ(), fc.clock, 5000*time.Millisecond)
		logger.Debugf("Launched %s - result %v %v %d", line, string(result.Stdout), string(result.Stderr), result.Code)
		if err != nil {
			return err
		}
	}
	// TODO(wpk) 2017-09-28 Although officially not needed we do fanctl up -a just to be sure -
	// fanatic sometimes fails to bring up interface because of some weird interactions with iptables.
	result, err := scriptrunner.RunCommand("fanctl up -a", os.Environ(), fc.clock, 5000*time.Millisecond)
	logger.Debugf("Launched fanctl up -a - result %v %v %d", string(result.Stdout), string(result.Stderr), result.Code)

	return err
}

func NewFanConfigurer(config FanConfigurerConfig, clock clock.Clock) (*FanConfigurer, error) {
	fc := &FanConfigurer{
		config: config,
		clock:  clock,
	}
	// We need to launch it once here to make sure that it's configured right away,
	// so that machiner will have a proper fan device address to report back
	// to controller.
	err := fc.processNewConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = catacomb.Invoke(catacomb.Plan{
		Site: &fc.catacomb,
		Work: fc.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return fc, nil
}

func (fc *FanConfigurer) loop() error {
	// In 4.0 fan-config, along with fanconfigurer facade has been removed.
	// If we've been migrated from 3.6 to 4.0 the agent will attempt to
	// use the new facade, but it will fail.
	// Part of the migration steps to 4.0 prevents a migration with fan-config
	// set, so the code below is dead code.
	configWatcher, err := fc.config.Facade.WatchForFanConfigChanges()
	if errors.Is(err, errors.NotImplemented) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	if err := fc.catacomb.Add(configWatcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-fc.catacomb.Dying():
			return fc.catacomb.ErrDying()
		case _, ok := <-configWatcher.Changes():
			if !ok {
				return errors.New("FAN configuration watcher closed")
			}
			if err = fc.processNewConfig(); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// Kill implements Worker.Kill()
func (fc *FanConfigurer) Kill() {
	fc.catacomb.Kill(nil)
}

// Wait implements Worker.Wait()
func (fc *FanConfigurer) Wait() error {
	return fc.catacomb.Wait()
}
