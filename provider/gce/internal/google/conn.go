// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	"fmt"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/googleapis/gax-go/v2/callctx"
	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	transporthttp "google.golang.org/api/transport/http"
)

// Connection provides methods for interacting with the GCE API. The
// methods are limited to those needed by the juju GCE provider.
type Connection struct {
	zones        *compute.ZonesClient
	instances    *compute.InstancesClient
	machineTypes *compute.MachineTypesClient
	disks        *compute.DisksClient
	firewalls    *compute.FirewallsClient
	networks     *compute.NetworksClient
	subnetworks  *compute.SubnetworksClient
	projects     *compute.ProjectsClient

	projectID string
}

// Connect authenticates using the provided credentials and opens a
// low-level connection to the GCE API for the Connection. Calling
// Connect after a successful connection has already been made will
// result in an error. All errors that happen while authenticating and
// connecting are returned by Connect.
func Connect(ctx context.Context, connCfg ConnectionConfig, creds *Credentials) (*Connection, error) {
	projects, err := newRESTClient(ctx, creds, connCfg.HTTPClient, compute.NewProjectsRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	zones, err := newRESTClient(ctx, creds, connCfg.HTTPClient, compute.NewZonesRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	instances, err := newRESTClient(ctx, creds, connCfg.HTTPClient, compute.NewInstancesRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	machineTypes, err := newRESTClient(ctx, creds, connCfg.HTTPClient, compute.NewMachineTypesRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	disks, err := newRESTClient(ctx, creds, connCfg.HTTPClient, compute.NewDisksRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	firewalls, err := newRESTClient(ctx, creds, connCfg.HTTPClient, compute.NewFirewallsRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	networks, err := newRESTClient(ctx, creds, connCfg.HTTPClient, compute.NewNetworksRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnetworks, err := newRESTClient(ctx, creds, connCfg.HTTPClient, compute.NewSubnetworksRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}

	conn := &Connection{
		projects:     projects,
		zones:        zones,
		instances:    instances,
		machineTypes: machineTypes,
		disks:        disks,
		firewalls:    firewalls,
		networks:     networks,
		subnetworks:  subnetworks,
		projectID:    connCfg.ProjectID,
	}
	return conn, nil
}

type newClientFunc[T any] func(ctx context.Context, opts ...option.ClientOption) (*T, error)

// newRESTClient opens a new low-level connection to the GCE API using
// the input credentials and returns it.
// This includes building the OAuth-wrapping network transport.
func newRESTClient[T any](ctx context.Context, creds *Credentials, httpClient *jujuhttp.Client, newClient newClientFunc[T]) (*T, error) {
	cfg, err := newJWTConfig(creds)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We're substituting the transport, with a wrapped GCE specific version of
	// the original http.Client.
	newHttpClient := *httpClient.Client()

	tsOpt := option.WithTokenSource(cfg.TokenSource(ctx))
	if newHttpClient.Transport, err = transporthttp.NewTransport(ctx, newHttpClient.Transport, tsOpt); err != nil {
		return nil, errors.Trace(err)
	}

	client, err := newClient(ctx,
		tsOpt,
		option.WithHTTPClient(&newHttpClient),
	)
	return client, errors.Trace(err)
}

func fetchResults[T any](iter func() (*T, error), badge string) ([]*T, error) {
	var results []*T
	for {
		item, err := iter()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Annotatef(err, "fetching %q", badge)
		}
		results = append(results, item)
	}
	return results, nil
}

func (c *Connection) getProjectServiceAccount(ctx context.Context, projectID string) (string, error) {
	callctx.SetHeaders(ctx, "x-goog-request-params", fmt.Sprintf("%s=%v", "fields", "defaultServiceAccount"))
	proj, err := c.projects.Get(ctx, &computepb.GetProjectRequest{
		Project: projectID,
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return proj.GetDefaultServiceAccount(), nil
}

// VerifyCredentials ensures that the authentication credentials used
// to connect are valid for use in the project and region defined for
// the Connection. If they are not then an error is returned.
func (c *Connection) VerifyCredentials(ctx context.Context) error {
	if _, err := c.getProjectServiceAccount(ctx, c.projectID); err != nil {
		// TODO(ericsnow) Wrap err with something about bad credentials?
		return errors.Trace(err)
	}
	return nil
}

// DefaultServiceAccount returns the default service account for the project.
func (c *Connection) DefaultServiceAccount(ctx context.Context) (string, error) {
	return c.getProjectServiceAccount(ctx, c.projectID)
}
