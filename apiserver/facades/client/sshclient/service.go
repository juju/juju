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

// ModelProviderService providers access to the model provider service.
type ModelProviderService interface {
	// GetCloudSpecForSSH returns the cloud spec for sshing into a k8s pod.
	GetCloudSpecForSSH(ctx context.Context) (cloudspec.CloudSpec, error)
}
