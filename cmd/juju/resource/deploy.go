// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/core/resources"
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

	ids, err = d.upload(args.ResourceValues, args.Revisions)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ids, nil
}

type osOpenFunc func(path string) (ReadSeekCloser, error)

type deployUploader struct {
	applicationID string
	chID          charmstore.CharmID
	csMac         *macaroon.Macaroon
	resources     map[string]charmresource.Meta
	client        DeployClient
	osOpen        osOpenFunc
	osStat        func(path string) error
}

func (d deployUploader) upload(resourceValues map[string]string, revisions map[string]int) (map[string]string, error) {
	if err := d.validateResources(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := d.checkExpectedResources(resourceValues, revisions); err != nil {
		return nil, errors.Trace(err)
	}

	if err := d.validateResourceDetails(resourceValues); err != nil {
		return nil, errors.Trace(err)
	}

	storeResources := d.charmStoreResources(resourceValues, revisions)
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

	for name, resValue := range resourceValues {
		r, err := OpenResource(resValue, d.resources[name].Type, d.osOpen)
		if err != nil {
			return nil, errors.Trace(err)
		}
		id, err := d.uploadPendingResource(name, resValue, r)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pending[name] = id
	}

	return pending, nil
}

func (d deployUploader) validateResourceDetails(res map[string]string) error {
	for name, value := range res {
		var err error
		switch d.resources[name].Type {
		case charmresource.TypeFile:
			err = d.checkFile(name, value)
		case charmresource.TypeContainerImage:
			var dockerDetails resources.DockerImageDetails
			dockerDetails, err = getDockerDetailsData(value, d.osOpen)
			if err != nil {
				return err
			}
			// At the moment this is the same validation that occurs in getDockerDetailsData
			err = resources.CheckDockerDetails(name, dockerDetails)
		default:
			return fmt.Errorf("unknown resource: %s", name)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (d deployUploader) checkFile(name, path string) error {
	err := d.osStat(path)
	if os.IsNotExist(err) {
		return errors.Annotatef(err, "file for resource %q", name)
	}
	if err != nil {
		return errors.Annotatef(err, "can't read file for resource %q", name)
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

// charmStoreResources returns which resources revisions will need to be retrieved
// either as they where explicitly requested by the user for that rev or they
// weren't provided by the user.
func (d deployUploader) charmStoreResources(uploads map[string]string, revisions map[string]int) []charmresource.Resource {
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

func (d deployUploader) uploadPendingResource(resourcename, resourcevalue string, data io.ReadSeeker) (id string, err error) {
	res := charmresource.Resource{
		Meta:   d.resources[resourcename],
		Origin: charmresource.OriginUpload,
	}

	return d.client.UploadPendingResource(d.applicationID, res, resourcevalue, data)
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

// getDockerDetailsData determines if path is a local file path and extracts the
// details from that otherwise path is considered to be a registry path.
func getDockerDetailsData(path string, osOpen osOpenFunc) (resources.DockerImageDetails, error) {
	f, err := osOpen(path)
	if err == nil {
		defer f.Close()
		details, err := unMarshalDockerDetails(f)
		if err != nil {
			return details, errors.Trace(err)
		}
		return details, nil
	} else if err := resources.ValidateDockerRegistryPath(path); err == nil {
		return resources.DockerImageDetails{
			RegistryPath: path,
		}, nil
	}
	return resources.DockerImageDetails{}, errors.NotValidf("filepath or registry path: %s", path)

}

func unMarshalDockerDetails(data io.Reader) (resources.DockerImageDetails, error) {
	var details resources.DockerImageDetails
	contents, err := ioutil.ReadAll(data)
	if err != nil {
		return details, errors.Trace(err)
	}

	if err := json.Unmarshal(contents, &details); err != nil {
		if err := yaml.Unmarshal(contents, &details); err != nil {
			return details, errors.Annotate(err, "file neither valid json or yaml")
		}
	}
	if err := resources.ValidateDockerRegistryPath(details.RegistryPath); err != nil {
		return resources.DockerImageDetails{}, err
	}
	return details, nil
}

func OpenResource(resValue string, resType charmresource.Type, osOpen osOpenFunc) (ReadSeekCloser, error) {
	switch resType {
	case charmresource.TypeFile:
		f, err := osOpen(resValue)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return f, nil
	case charmresource.TypeContainerImage:
		dockerDetails, err := getDockerDetailsData(resValue, osOpen)
		if err != nil {
			return nil, errors.Trace(err)
		}
		data, err := yaml.Marshal(dockerDetails)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return noopCloser{bytes.NewReader(data)}, nil
	default:
		return nil, errors.Errorf("unknown resource type %q", resType)
	}
}

type noopCloser struct {
	io.ReadSeeker
}

func (noopCloser) Close() error {
	return nil
}
