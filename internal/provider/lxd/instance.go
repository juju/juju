// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"

	"github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/container/lxd"
)

type environInstance struct {
	container *lxd.Container
	env       *environ
}

var _ instances.Instance = (*environInstance)(nil)

func newInstance(container *lxd.Container, env *environ) *environInstance {
	return &environInstance{
		container: container,
		env:       env,
	}
}

// Id implements instances.Instance.
func (i *environInstance) Id() instance.Id {
	return instance.Id(i.container.Name)
}

// Status implements instances.Instance.
func (i *environInstance) Status(ctx context.Context) instance.Status {
	var jujuStatus status.Status
	code := i.container.StatusCode
	switch code {
	case api.Starting, api.Started:
		jujuStatus = status.Allocating
	case api.Running:
		jujuStatus = status.Running
	case api.Freezing, api.Frozen, api.Thawed, api.Stopping, api.Stopped:
		jujuStatus = status.Empty
	default:
		jujuStatus = status.Empty
	}
	return instance.Status{
		Status:  jujuStatus,
		Message: code.String(),
	}

}

// Addresses implements instances.Instance.
func (i *environInstance) Addresses(_ context.Context) (network.ProviderAddresses, error) {
	addrs, err := i.env.server().ContainerAddresses(i.container.Name)
	return addrs, errors.Trace(err)
}
