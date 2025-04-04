// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"context"

	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
}

// StubService will be replaced once the implementation is finished.
type StubService interface {
	// CloudSpec returns the cloud spec for the model.
	CloudSpec(ctx context.Context) (cloudspec.CloudSpec, error)
}

// ExecService will be replaced once the implementation is finished.
type ExecService interface {
	// GetCAASUnitExecSecretToken returns a token that can be used to run exec operations
	// on the provider cloud.
	GetCAASUnitExecSecretToken(ctx context.Context) (string, error)
}
