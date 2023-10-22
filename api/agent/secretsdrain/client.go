// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

import (
	"github.com/juju/juju/api/base"
	commonsecretdrain "github.com/juju/juju/api/common/secretsdrain"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// NewClient creates a secrets api client.
func NewClient(caller base.APICaller, options ...Option) *commonsecretdrain.Client {
	return commonsecretdrain.NewClient(base.NewFacadeCaller(caller, "SecretsDrain", options...))
}
