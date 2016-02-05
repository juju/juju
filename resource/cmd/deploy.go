package cmd

import (
	"io"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/api"
)

var logger = loggo.GetLogger("resource/cmd")

type uploadClient interface {
	AddPendingResources(serviceID string, resources []charmresource.Resource) (ids []string, err error)
	AddPendingResource(serviceID string, resource charmresource.Resource, r io.Reader) (id string, err error)
}

type deployUploader struct {
	serviceID string
	resources map[string]charmresource.Meta
	client    uploadClient
	osOpen    func(path string) (io.ReadCloser, error)
}

// Upload uploads the bytes and metadata for the given resourcename - filename
// pairs, returning a map of resource name to uniqueIDs, to be passed along with
// the ServiceDeploy API command.
func Upload(serviceId string, files map[string]string, resources map[string]charmresource.Meta, api api.Connection) (ids map[string]string, err error) {
	// TODO: hook up with
	return nil, nil

	d := deployUploader{
		serviceID: serviceId,
		client:    nil,
		resources: resources,
		osOpen:    func(s string) (io.ReadCloser, error) { return os.Open(s) },
	}

	// TODO(natefinch): hook up with uploadClient code!
	return d.upload(files)
}

func (d deployUploader) upload(files map[string]string) (map[string]string, error) {
	if err := d.checkExpectedResources(files); err != nil {
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

func (d deployUploader) storeResources(uploads map[string]string) []charmresource.Resource {
	var resources []charmresource.Resource
	for name, meta := range d.resources {
		if _, ok := uploads[name]; !ok {
			resources = append(resources, charmresource.Resource{
				Meta:   meta,
				Origin: charmresource.OriginStore,
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
