// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"context"
	"os"

	"github.com/juju/cmd/v4"

	"github.com/juju/juju/environs"
)

type interruptable interface {
	InterruptNotify(c chan<- os.Signal)
	StopInterruptNotify(c chan<- os.Signal)
}

type bootstrapContext struct {
	context.Context
	environs.BootstrapLogger
	interruptable
	verifyCredentials bool
}

// ShouldVerifyCredentials implements BootstrapContext.ShouldVerifyCredentials
func (c *bootstrapContext) ShouldVerifyCredentials() bool {
	return c.verifyCredentials
}

// BootstrapContext returns a new BootstrapContext constructed from a command Context.
func BootstrapContext(ctx context.Context, cmdContext *cmd.Context) environs.BootstrapContext {
	return &bootstrapContext{
		Context:           ctx,
		interruptable:     cmdContext,
		BootstrapLogger:   cmdContext,
		verifyCredentials: true,
	}
}

// BootstrapContextNoVerify returns a new BootstrapContext constructed from a command Context
// where the validation of credentials is false.
func BootstrapContextNoVerify(ctx context.Context, cmdContext *cmd.Context) environs.BootstrapContext {
	return &bootstrapContext{
		Context:           ctx,
		interruptable:     cmdContext,
		BootstrapLogger:   cmdContext,
		verifyCredentials: false,
	}
}
