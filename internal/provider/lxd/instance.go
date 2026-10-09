// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"maps"
	"slices"

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
func (i *environInstance) Addresses(ctx context.Context) (network.ProviderAddresses, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	srv := i.env.server()
	addrs, err := srv.ContainerAddresses(i.container.Name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	forwardAddresses, err := ovnForwardAddresses(ctx, srv, i.container.Name)
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving OVN forward addresses for instance %q", i.container.Name)
	}
	for _, iface := range slices.Sorted(maps.Keys(forwardAddresses)) {
		addrs = append(addrs, forwardAddresses[iface]...)
	}
	return addrs, nil
}
