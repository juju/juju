// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"context"

	"github.com/juju/cmd/v3"

	"github.com/juju/juju/environs"
)

// Define a type alias so we can embed *cmd.Context and have a Context() method.
type cmdContext = cmd.Context

type bootstrapContext struct {
	*cmdContext
	verifyCredentials bool
	ctx               context.Context
}

// ShouldVerifyCredentials implements BootstrapContext.ShouldVerifyCredentials
func (c *bootstrapContext) ShouldVerifyCredentials() bool {
	return c.verifyCredentials
}

// Context returns this bootstrap's context.Context value.
func (c *bootstrapContext) Context() context.Context {
	return c.ctx
}

// BootstrapContext returns a new BootstrapContext constructed from a command Context.
func BootstrapContext(ctx context.Context, cmdContext *cmd.Context) environs.BootstrapContext {
	return &bootstrapContext{
		cmdContext:        cmdContext,
		verifyCredentials: true,
		ctx:               ctx,
	}
}

// BootstrapContextNoVerify returns a new BootstrapContext constructed from a command Context
// where the validation of credentials is false.
func BootstrapContextNoVerify(ctx context.Context, cmdContext *cmd.Context) environs.BootstrapContext {
	return &bootstrapContext{
		cmdContext:        cmdContext,
		verifyCredentials: false,
		ctx:               ctx,
	}
}
