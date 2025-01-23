// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/client"
	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/common/charms"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

// StatusClientAPI defines status related APIs.
type StatusClientAPI interface {
	Status(ctx context.Context, args *client.StatusArgs) (*params.FullStatus, error)
	Close() error
}

// CloudCredentialAPI defines cloud credential related APIs.
type CloudCredentialAPI interface {
	Cloud(tag names.CloudTag) (jujucloud.Cloud, error)
	CredentialContents(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error)
	BestAPIVersion() int
	Close() error
}

// ApplicationAPI defines application related APIs.
type ApplicationAPI interface {
	Leader(context.Context, string) (string, error)
	Close() error
	UnitsInfo(ctx context.Context, units []names.UnitTag) ([]application.UnitInfo, error)
	GetCharmURLOrigin(ctx context.Context, applicationName string) (*charm.URL, apicharm.Origin, error)
}

// CharmAPI defines charm related APIs.
type CharmAPI interface {
	CharmInfo(ctx context.Context, charmURL string) (*charms.CharmInfo, error)
	Close() error
}

type coreSSHClient interface {
	PublicAddress(ctx context.Context, target string) (string, error)
	PrivateAddress(ctx context.Context, target string) (string, error)
	AllAddresses(ctx context.Context, target string) ([]string, error)
	PublicKeys(ctx context.Context, target string) ([]string, error)
	Proxy(ctx context.Context) (bool, error)
	Close() error
}

// SSHClientAPI defines ssh client related APIs.
type SSHClientAPI interface {
	coreSSHClient
	ModelCredentialForSSH(ctx context.Context) (cloudspec.CloudSpec, error)
}

// SSHControllerAPI defines controller related APIs.
type SSHControllerAPI interface {
	ControllerConfig(context.Context) (controller.Config, error)
}
