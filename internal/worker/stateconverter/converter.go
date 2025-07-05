// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	jujuagent "github.com/juju/juju/agent"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	internalerrors "github.com/juju/juju/internal/errors"
)

type config struct {
	machineTag  names.MachineTag
	machiner    Machiner
	agentClient Agent
	agent       jujuagent.Agent
	logger      logger.Logger
}

// NewConverter returns a new notify watch handler that will convert the given machine &
// agent to a controller.
func NewConverter(cfg config) watcher.NotifyHandler {
	return &converter{
		machiner:    cfg.machiner,
		machineTag:  cfg.machineTag,
		agentClient: cfg.agentClient,
		agent:       cfg.agent,
		logger:      cfg.logger,
	}
}

// converter is a NotifyWatchHandler that converts a unit hosting machine to a
// state machine.
type converter struct {
	machineTag  names.MachineTag
	machiner    Machiner
	machine     Machine
	agentClient Agent
	agent       jujuagent.Agent
	logger      logger.Logger
}

// wrapper is a wrapper around api/machiner.State to match the (local) machiner
// interface.
type wrapper struct {
	m *machiner.Client
}

// Machine implements machiner.Machine and returns a machine from the wrapper
// api/machiner.
func (w wrapper) Machine(ctx context.Context, tag names.MachineTag) (Machine, error) {
	m, err := w.m.Machine(ctx, tag)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// SetUp implements NotifyWatchHandler's SetUp method. It returns a watcher that
// checks for changes to the current machine.
func (c *converter) SetUp(ctx context.Context) (watcher.NotifyWatcher, error) {
	c.logger.Tracef(ctx, "Calling SetUp for %s", c.machineTag)
	m, err := c.machiner.Machine(ctx, c.machineTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.machine = m
	return m.Watch(ctx)
}

// Handle implements NotifyWatchHandler's Handle method.  If the change means
// that the machine is now expected to manage the environment, we throw a fatal
// error to instigate agent restart.
func (c *converter) Handle(ctx context.Context) error {
	c.logger.Tracef(ctx, "Calling Handle for %s", c.machineTag)
	isController, err := c.machine.IsController(ctx, c.machineTag.Id())
	if err != nil {
		return errors.Trace(err)
	}

	if !isController {
		return nil
	}

	// If the machine needs Client, grab the state serving info
	// over the API and write it to the agent configuration.
	info, err := c.agentClient.StateServingInfo(ctx)
	if err != nil {
		return internalerrors.Errorf("getting state serving info: %w", err)
	}

	err = c.agent.ChangeConfig(func(config jujuagent.ConfigSetter) error {
		_, hasInfo := config.StateServingInfo()
		if hasInfo {
			return nil
		}

		config.SetStateServingInfo(info)
		return nil
	})
	if err != nil {
		return errors.Annotatef(err, "setting state serving info for %s", c.machineTag)
	}

	return fmt.Errorf("bounce agent to pick up new jobs%w", errors.Hide(agenterrors.FatalError))
}

// TearDown implements NotifyWatchHandler's TearDown method.
func (c *converter) TearDown() error {
	return nil
}
