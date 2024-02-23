// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"io"

	"github.com/juju/errors"
	charmresource "github.com/juju/juju/charm/resource"

	apiresources "github.com/juju/juju/api/client/resources"
	"github.com/juju/juju/cmd/modelcmd"
)

// DeployClient exposes the functionality of the resources API needed
// for deploy.
type DeployClient interface {
	// AddPendingResources adds pending metadata for store-based resources.
	AddPendingResources(applicationID string, chID apiresources.CharmID, resources []charmresource.Resource) (ids []string, err error)

	// UploadPendingResource uploads data and metadata for a pending resource for the given application.
	UploadPendingResource(ctx context.Context, applicationID string, resource charmresource.Resource, filename string, r io.ReadSeeker) (id string, err error)
}

// DeployResourcesArgs holds the arguments to DeployResources().
type DeployResourcesArgs struct {
	// ApplicationID identifies the application being deployed.
	ApplicationID string

	// CharmID identifies the application's charm.
	CharmID apiresources.CharmID

	// ResourceValues is the set of resources for which a value
	// was provided at the command-line.
	ResourceValues map[string]string

	// Revisions is the set of resources for which a revision
	// was provided at the command-line.
	Revisions map[string]int

	// ResourcesMeta holds the charm metadata for each of the resources
	// that should be added/updated on the controller.
	ResourcesMeta map[string]charmresource.Meta

	// Client is the resources API client to use during deploy.
	Client DeployClient

	// Filesystem provides access to the filesystem.
	Filesystem modelcmd.Filesystem
}

// DeployResources uploads the bytes for the given files to the server and
// creates pending resource metadata for the all resource mentioned in the
// metadata. It returns a map of resource name to pending resource IDs.
func DeployResources(ctx context.Context, args DeployResourcesArgs) (ids map[string]string, err error) {
	d := deployUploader{
		applicationID: args.ApplicationID,
		chID:          args.CharmID,
		client:        args.Client,
		resources:     args.ResourcesMeta,
		filesystem:    args.Filesystem,
	}

	ids, err = d.upload(ctx, args.ResourceValues, args.Revisions)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ids, nil
}

type deployUploader struct {
	applicationID string
	chID          apiresources.CharmID
	resources     map[string]charmresource.Meta
	client        DeployClient
	filesystem    modelcmd.Filesystem
}

func (d deployUploader) upload(ctx context.Context, resourceValues map[string]string, revisions map[string]int) (map[string]string, error) {
	if err := ValidateResources(d.resources); err != nil {
		return nil, errors.Trace(err)
	}

	if err := CheckExpectedResources(resourceValues, revisions, d.resources); err != nil {
		return nil, errors.Trace(err)
	}

	if err := ValidateResourceDetails(resourceValues, d.resources, d.filesystem); err != nil {
		return nil, errors.Trace(err)
	}

	storeResources := d.storeResources(resourceValues, revisions)
	pending := map[string]string{}
	if len(storeResources) > 0 {

		ids, err := d.client.AddPendingResources(d.applicationID, d.chID, storeResources)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// guaranteed 1:1 correlation between ids and resources.
		for i, res := range storeResources {
			pending[res.Name] = ids[i]
		}
	}

	for name, resValue := range resourceValues {
		r, err := OpenResource(resValue, d.resources[name].Type, d.filesystem.Open)
		if err != nil {
			return nil, errors.Annotatef(err, "resource %q", name)
		}
		id, err := d.uploadPendingResource(ctx, name, resValue, r)
		if err != nil {
			return nil, errors.Annotatef(err, "resource %q", name)
		}
		pending[name] = id
	}

	return pending, nil
}

// storeResources returns which resources revisions will need to be retrieved
// either as they were explicitly requested by the user for that rev or they
// weren't provided by the user.
func (d deployUploader) storeResources(uploads map[string]string, revisions map[string]int) []charmresource.Resource {
	var resources []charmresource.Resource
	for name, meta := range d.resources {
		if _, ok := uploads[name]; ok {
			continue
		}

		revision := -1
		if rev, ok := revisions[name]; ok {
			revision = rev
		}

		resources = append(resources, charmresource.Resource{
			Meta:     meta,
			Origin:   charmresource.OriginStore,
			Revision: revision,
			// Fingerprint and Size will be added server-side in
			// the AddPendingResources() API call.
		})
	}
	return resources
}

func (d deployUploader) uploadPendingResource(ctx context.Context, resourcename, resourcevalue string, data io.ReadSeeker) (id string, err error) {
	res := charmresource.Resource{
		Meta:   d.resources[resourcename],
		Origin: charmresource.OriginUpload,
	}

	return d.client.UploadPendingResource(ctx, d.applicationID, res, resourcevalue, data)
}
