// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	"fmt"
	"iter"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/compute/metadata"
	"github.com/googleapis/gax-go/v2/callctx"
	"github.com/juju/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	transporthttp "google.golang.org/api/transport/http"

	jujuhttp "github.com/juju/juju/internal/http"
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
	tokenSource, projectID, err := tokenSourceFromCreds(ctx, connCfg.HTTPClient, creds)
	if err != nil {
		return nil, errors.Trace(err)
	}
	projects, err := newRESTClient(ctx, tokenSource, connCfg.HTTPClient, compute.NewProjectsRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	zones, err := newRESTClient(ctx, tokenSource, connCfg.HTTPClient, compute.NewZonesRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	instances, err := newRESTClient(ctx, tokenSource, connCfg.HTTPClient, compute.NewInstancesRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	machineTypes, err := newRESTClient(ctx, tokenSource, connCfg.HTTPClient, compute.NewMachineTypesRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	disks, err := newRESTClient(ctx, tokenSource, connCfg.HTTPClient, compute.NewDisksRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	firewalls, err := newRESTClient(ctx, tokenSource, connCfg.HTTPClient, compute.NewFirewallsRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	networks, err := newRESTClient(ctx, tokenSource, connCfg.HTTPClient, compute.NewNetworksRESTClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnetworks, err := newRESTClient(ctx, tokenSource, connCfg.HTTPClient, compute.NewSubnetworksRESTClient)
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
		projectID:    projectID,
	}
	return conn, nil
}

func tokenSourceFromCreds(ctx context.Context, httpClient *jujuhttp.Client, creds *Credentials) (oauth2.TokenSource, string, error) {
	// If we're using a service account, get the token from the metadata service.
	if creds.ServiceAccount != "" {
		meta := metadata.NewClient(httpClient.Client())
		projectID, err := meta.ProjectIDWithContext(ctx)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
		ts := google.ComputeTokenSource(creds.ServiceAccount, Scopes...)
		return ts, projectID, nil
	}

	cfg, err := newJWTConfig(creds)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	ts := cfg.TokenSource(ctx)
	return ts, creds.ProjectID, nil
}

type newClientFunc[T any] func(ctx context.Context, opts ...option.ClientOption) (*T, error)

// newRESTClient opens a new low-level connection to the GCE API using
// the input credentials and returns it.
// This includes building the OAuth-wrapping network transport.
func newRESTClient[T any](ctx context.Context, tokenSource oauth2.TokenSource, httpClient *jujuhttp.Client, newClient newClientFunc[T]) (*T, error) {
	// We're substituting the transport, with a wrapped GCE specific version of
	// the original http.Client.
	newHttpClient := *httpClient.Client()

	tsOpt := option.WithTokenSource(tokenSource)
	var err error
	if newHttpClient.Transport, err = transporthttp.NewTransport(ctx, newHttpClient.Transport, tsOpt); err != nil {
		return nil, errors.Trace(err)
	}

	client, err := newClient(ctx,
		tsOpt,
		option.WithHTTPClient(&newHttpClient),
	)
	return client, errors.Trace(err)
}

func fetchResults[T any](allItems iter.Seq2[*T, error], badge string, matchers ...func(*T) bool) ([]*T, error) {
	var results []*T
	for item, err := range allItems {
		if err != nil {
			return nil, errors.Annotatef(err, "fetching %q", badge)
		}
		include := len(matchers) == 0
		for _, m := range matchers {
			if m(item) {
				include = true
				break
			}
		}
		if include {
			results = append(results, item)
		}
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
