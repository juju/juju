// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"io"
	"os"
	"strings"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

// DeployClient exposes the functionality of the resources API needed
// for deploy.
type DeployClient interface {
	// AddPendingResources adds pending metadata for store-based resources.
	AddPendingResources(serviceID string, resources []charmresource.Resource) (ids []string, err error)
	// AddPendingResource uploads data and metadata for a pending resource for the given service.
	AddPendingResource(serviceID string, resource charmresource.Resource, r io.ReadSeeker) (id string, err error)
}

// DeployResources uploads the bytes for the given files to the server and
// creates pending resource metadata for the all resource mentioned in the
// metadata. It returns a map of resource name to pending resource IDs.
func DeployResources(serviceID string, files map[string]string, resources map[string]charmresource.Meta, client DeployClient) (ids map[string]string, err error) {
	d := deployUploader{
		serviceID: serviceID,
		client:    client,
		resources: resources,
		osOpen:    func(s string) (ReadSeekCloser, error) { return os.Open(s) },
		osStat:    func(s string) error { _, err := os.Stat(s); return err },
	}

	ids, err = d.upload(files)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ids, nil
}

type deployUploader struct {
	serviceID string
	resources map[string]charmresource.Meta
	client    DeployClient
	osOpen    func(path string) (ReadSeekCloser, error)
	osStat    func(path string) error
}

func (d deployUploader) upload(files map[string]string) (map[string]string, error) {
	if err := d.validateResources(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := d.checkExpectedResources(files); err != nil {
		return nil, errors.Trace(err)
	}

	if err := d.checkFiles(files); err != nil {
		return nil, errors.Trace(err)
	}

	storeResources := d.storeResources(files)
	pending := map[string]string{}
	if len(storeResources) > 0 {
		ids, err := d.client.AddPendingResources(d.serviceID, storeResources)
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

func (d deployUploader) storeResources(uploads map[string]string) []charmresource.Resource {
	var resources []charmresource.Resource
	for name, meta := range d.resources {
		if _, ok := uploads[name]; !ok {
			resources = append(resources, charmresource.Resource{
				Meta:   meta,
				Origin: charmresource.OriginStore,
				// Revision, Fingerprint, and Size will be added server-side,
				// when we download the bytes from the store.
			})
		}
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

	id, err = d.client.AddPendingResource(d.serviceID, res, f)
	if err != nil {
		return "", errors.Trace(err)
	}
	return id, err
}

func (d deployUploader) checkExpectedResources(provided map[string]string) error {
	var unknown []string

	for name := range provided {
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
