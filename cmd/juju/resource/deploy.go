// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"io"
	"os"
	"strings"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6/resource"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/charmstore"
)

// DeployClient exposes the functionality of the resources API needed
// for deploy.
type DeployClient interface {
	// AddPendingResources adds pending metadata for store-based resources.
	AddPendingResources(applicationID string, chID charmstore.CharmID, csMac *macaroon.Macaroon, resources []charmresource.Resource) (ids []string, err error)

	// UploadPendingResource uploads data and metadata for a pending resource for the given application.
	UploadPendingResource(applicationID string, resource charmresource.Resource, filename string, r io.ReadSeeker) (id string, err error)
}

// DeployResourcesArgs holds the arguments to DeployResources().
type DeployResourcesArgs struct {
	// ApplicationID identifies the application being deployed.
	ApplicationID string

	// CharmID identifies the application's charm.
	CharmID charmstore.CharmID

	// CharmStoreMacaroon is the macaroon to use for the charm when
	// interacting with the charm store.
	CharmStoreMacaroon *macaroon.Macaroon

	// Filenames is the set of resources for which a filename
	// was provided at the command-line.
	Filenames map[string]string

	// Revisions is the set of resources for which a revision
	// was provided at the command-line.
	Revisions map[string]int

	// ResourcesMeta holds the charm metadata for each of the resources
	// that should be added/updated on the controller.
	ResourcesMeta map[string]charmresource.Meta

	// Client is the resources API client to use during deploy.
	Client DeployClient
}

// DeployResources uploads the bytes for the given files to the server and
// creates pending resource metadata for the all resource mentioned in the
// metadata. It returns a map of resource name to pending resource IDs.
func DeployResources(args DeployResourcesArgs) (ids map[string]string, err error) {
	d := deployUploader{
		applicationID: args.ApplicationID,
		chID:          args.CharmID,
		csMac:         args.CharmStoreMacaroon,
		client:        args.Client,
		resources:     args.ResourcesMeta,
		osOpen:        func(s string) (ReadSeekCloser, error) { return os.Open(s) },
		osStat:        func(s string) error { _, err := os.Stat(s); return err },
	}

	ids, err = d.upload(args.Filenames, args.Revisions)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ids, nil
}

type deployUploader struct {
	applicationID string
	chID          charmstore.CharmID
	csMac         *macaroon.Macaroon
	resources     map[string]charmresource.Meta
	client        DeployClient
	osOpen        func(path string) (ReadSeekCloser, error)
	osStat        func(path string) error
}

func (d deployUploader) upload(files map[string]string, revisions map[string]int) (map[string]string, error) {
	if err := d.validateResources(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := d.checkExpectedResources(files, revisions); err != nil {
		return nil, errors.Trace(err)
	}

	if err := d.checkFiles(files); err != nil {
		return nil, errors.Trace(err)
	}

	storeResources := d.storeResources(files, revisions)
	pending := map[string]string{}
	if len(storeResources) > 0 {
		ids, err := d.client.AddPendingResources(d.applicationID, d.chID, d.csMac, storeResources)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// guaranteed 1:1 correlation between ids and resources.
		for i, res := range storeResources {
			pending[res.Name] = ids[i]
		}
	}

	for name, filename := range files {
		id, err := d.uploadFile(name, filename)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pending[name] = id
	}

	return pending, nil
}

func (d deployUploader) checkFiles(files map[string]string) error {
	for name, path := range files {
		err := d.osStat(path)
		if os.IsNotExist(err) {
			return errors.Annotatef(err, "file for resource %q", name)
		}
		if err != nil {
			return errors.Annotatef(err, "can't read file for resource %q", name)
		}
	}
	return nil
}

func (d deployUploader) validateResources() error {
	var errs []error
	for _, meta := range d.resources {
		if err := meta.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 1 {
		return errors.Trace(errs[0])
	}
	if len(errs) > 1 {
		msgs := make([]string, len(errs))
		for i, err := range errs {
			msgs[i] = err.Error()
		}
		return errors.NewNotValid(nil, strings.Join(msgs, ", "))
	}
	return nil
}

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

func (d deployUploader) uploadFile(resourcename, filename string) (id string, err error) {
	f, err := d.osOpen(filename)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer f.Close()
	res := charmresource.Resource{
		Meta:   d.resources[resourcename],
		Origin: charmresource.OriginUpload,
	}

	id, err = d.client.UploadPendingResource(d.applicationID, res, filename, f)
	if err != nil {
		return "", errors.Trace(err)
	}
	return id, err
}

func (d deployUploader) checkExpectedResources(filenames map[string]string, revisions map[string]int) error {
	var unknown []string
	for name := range filenames {
		if _, ok := d.resources[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	for name := range revisions {
		if _, ok := d.resources[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 1 {
		return errors.Errorf("unrecognized resource %q", unknown[0])
	}
	if len(unknown) > 1 {
		return errors.Errorf("unrecognized resources: %s", strings.Join(unknown, ", "))
	}
	return nil
}
