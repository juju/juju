// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/juju/charm/v7"
	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/juju/core/resources"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/charmstore"
)

type DeploySuite struct {
	testing.IsolationSuite

	stub *testing.Stub
}

var _ = gc.Suite(&DeploySuite{})

func (s *DeploySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
}

func (s DeploySuite) TestDeployResourcesWithoutFiles(c *gc.C) {
	deps := uploadDeps{s.stub, nil}
	cURL := charm.MustParseURL("cs:~a-user/trusty/spam-5")
	chID := charmstore.CharmID{
		URL: cURL,
	}
	csMac := &macaroon.Macaroon{}
	resources := map[string]charmresource.Meta{
		"store-tarball": {
			Name: "store-tarball",
			Type: charmresource.TypeFile,
			Path: "store.tgz",
		},
		"store-zip": {
			Name: "store-zip",
			Type: charmresource.TypeFile,
			Path: "store.zip",
		},
	}

	ids, err := DeployResources(DeployResourcesArgs{
		ApplicationID:      "mysql",
		CharmID:            chID,
		CharmStoreMacaroon: csMac,
		ResourceValues:     nil,
		Client:             deps,
		ResourcesMeta:      resources,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ids, gc.DeepEquals, map[string]string{
		"store-tarball": "id-store-tarball",
		"store-zip":     "id-store-zip",
	})

	s.stub.CheckCallNames(c, "AddPendingResources")
	s.stub.CheckCall(c, 0, "AddPendingResources", "mysql", chID, csMac, []charmresource.Resource{{
		Meta:     resources["store-tarball"],
		Origin:   charmresource.OriginStore,
		Revision: -1,
	}, {
		Meta:     resources["store-zip"],
		Origin:   charmresource.OriginStore,
		Revision: -1,
	}})
}

func (s DeploySuite) TestUploadFilesOnly(c *gc.C) {
	deps := uploadDeps{s.stub, []byte("file contents")}
	cURL := charm.MustParseURL("cs:~a-user/trusty/spam-5")
	chID := charmstore.CharmID{
		URL: cURL,
	}
	csMac := &macaroon.Macaroon{}
	du := deployUploader{
		applicationID: "mysql",
		chID:          chID,
		csMac:         csMac,
		client:        deps,
		resources: map[string]charmresource.Meta{
			"upload": {
				Name: "upload",
				Type: charmresource.TypeFile,
				Path: "upload",
			},
			"store": {
				Name: "store",
				Type: charmresource.TypeFile,
				Path: "store",
			},
		},
		osOpen: deps.Open,
		osStat: deps.Stat,
	}

	files := map[string]string{
		"upload": "foobar.txt",
	}
	revisions := map[string]int{}
	ids, err := du.upload(files, revisions)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ids, gc.DeepEquals, map[string]string{
		"upload": "id-upload",
		"store":  "id-store",
	})

	s.stub.CheckCallNames(c, "Stat", "AddPendingResources", "Open", "UploadPendingResource")
	expectedStore := []charmresource.Resource{
		{
			Meta:     du.resources["store"],
			Origin:   charmresource.OriginStore,
			Revision: -1,
		},
	}
	s.stub.CheckCall(c, 1, "AddPendingResources", "mysql", chID, csMac, expectedStore)
	s.stub.CheckCall(c, 2, "Open", "foobar.txt")

	expectedUpload := charmresource.Resource{
		Meta:   du.resources["upload"],
		Origin: charmresource.OriginUpload,
	}
	s.stub.CheckCall(c, 3, "UploadPendingResource", "mysql", expectedUpload, "foobar.txt", "file contents")
}

func (s DeploySuite) TestUploadRevisionsOnly(c *gc.C) {
	deps := uploadDeps{s.stub, nil}
	cURL := charm.MustParseURL("cs:~a-user/trusty/spam-5")
	chID := charmstore.CharmID{
		URL: cURL,
	}
	csMac := &macaroon.Macaroon{}
	du := deployUploader{
		applicationID: "mysql",
		chID:          chID,
		csMac:         csMac,
		client:        deps,
		resources: map[string]charmresource.Meta{
			"upload": {
				Name: "upload",
				Type: charmresource.TypeFile,
				Path: "upload",
			},
			"store": {
				Name: "store",
				Type: charmresource.TypeFile,
				Path: "store",
			},
		},
		osOpen: deps.Open,
		osStat: deps.Stat,
	}

	files := map[string]string{}
	revisions := map[string]int{
		"store": 3,
	}
	ids, err := du.upload(files, revisions)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ids, gc.DeepEquals, map[string]string{
		"upload": "id-upload",
		"store":  "id-store",
	})

	s.stub.CheckCallNames(c, "AddPendingResources")
	expectedStore := []charmresource.Resource{{
		Meta:     du.resources["store"],
		Origin:   charmresource.OriginStore,
		Revision: 3,
	}, {
		Meta:     du.resources["upload"],
		Origin:   charmresource.OriginStore,
		Revision: -1,
	}}
	s.stub.CheckCall(c, 0, "AddPendingResources", "mysql", chID, csMac, expectedStore)
}

func (s DeploySuite) TestUploadFilesAndRevisions(c *gc.C) {
	deps := uploadDeps{s.stub, []byte("file contents")}
	cURL := charm.MustParseURL("cs:~a-user/trusty/spam-5")
	chID := charmstore.CharmID{
		URL: cURL,
	}
	csMac := &macaroon.Macaroon{}
	du := deployUploader{
		applicationID: "mysql",
		chID:          chID,
		csMac:         csMac,
		client:        deps,
		resources: map[string]charmresource.Meta{
			"upload": {
				Name: "upload",
				Type: charmresource.TypeFile,
				Path: "upload",
			},
			"store": {
				Name: "store",
				Type: charmresource.TypeFile,
				Path: "store",
			},
		},
		osOpen: deps.Open,
		osStat: deps.Stat,
	}

	files := map[string]string{
		"upload": "foobar.txt",
	}
	revisions := map[string]int{
		"store": 3,
	}
	ids, err := du.upload(files, revisions)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ids, gc.DeepEquals, map[string]string{
		"upload": "id-upload",
		"store":  "id-store",
	})

	s.stub.CheckCallNames(c, "Stat", "AddPendingResources", "Open", "UploadPendingResource")
	expectedStore := []charmresource.Resource{
		{
			Meta:     du.resources["store"],
			Origin:   charmresource.OriginStore,
			Revision: 3,
		},
	}
	s.stub.CheckCall(c, 1, "AddPendingResources", "mysql", chID, csMac, expectedStore)
	s.stub.CheckCall(c, 2, "Open", "foobar.txt")

	expectedUpload := charmresource.Resource{
		Meta:   du.resources["upload"],
		Origin: charmresource.OriginUpload,
	}
	s.stub.CheckCall(c, 3, "UploadPendingResource", "mysql", expectedUpload, "foobar.txt", "file contents")
}

func (s DeploySuite) TestUploadUnexpectedResourceFile(c *gc.C) {
	deps := uploadDeps{s.stub, nil}
	du := deployUploader{
		applicationID: "mysql",
		client:        deps,
		resources: map[string]charmresource.Meta{
			"res1": {
				Name: "res1",
				Type: charmresource.TypeFile,
				Path: "path",
			},
		},
		osOpen: deps.Open,
		osStat: deps.Stat,
	}

	files := map[string]string{"some bad resource": "foobar.txt"}
	revisions := map[string]int{}
	_, err := du.upload(files, revisions)
	c.Check(err, gc.ErrorMatches, `unrecognized resource "some bad resource"`)

	s.stub.CheckNoCalls(c)
}

func (s DeploySuite) TestUploadUnexpectedResourceRevision(c *gc.C) {
	deps := uploadDeps{s.stub, nil}
	du := deployUploader{
		applicationID: "mysql",
		client:        deps,
		resources: map[string]charmresource.Meta{
			"res1": {
				Name: "res1",
				Type: charmresource.TypeFile,
				Path: "path",
			},
		},
		osOpen: deps.Open,
		osStat: deps.Stat,
	}

	files := map[string]string{}
	revisions := map[string]int{"some bad resource": 2}
	_, err := du.upload(files, revisions)
	c.Check(err, gc.ErrorMatches, `unrecognized resource "some bad resource"`)

	s.stub.CheckNoCalls(c)
}

func (s DeploySuite) TestMissingResource(c *gc.C) {
	deps := uploadDeps{s.stub, nil}
	du := deployUploader{
		applicationID: "mysql",
		client:        deps,
		resources: map[string]charmresource.Meta{
			"res1": {
				Name: "res1",
				Type: charmresource.TypeFile,
				Path: "path",
			},
		},
		osOpen: deps.Open,
		osStat: deps.Stat,
	}

	// set the error that will be returned by os.Stat
	s.stub.SetErrors(os.ErrNotExist)

	files := map[string]string{"res1": "foobar.txt"}
	revisions := map[string]int{}
	_, err := du.upload(files, revisions)
	c.Check(err, gc.ErrorMatches, `file for resource "res1".*`)
	c.Check(errors.Cause(err), jc.Satisfies, os.IsNotExist)
}

func (s DeploySuite) TestDeployDockerResourceRegistryPathString(c *gc.C) {
	deps := uploadDeps{s.stub, nil}
	cURL := charm.MustParseURL("cs:~a-user/mysql-k8s-5")
	chID := charmstore.CharmID{
		URL: cURL,
	}
	csMac := &macaroon.Macaroon{}
	resourceMeta := map[string]charmresource.Meta{
		"mysql_image": {
			Name: "mysql_image",
			Type: charmresource.TypeContainerImage,
		},
	}

	passedResourceValues := map[string]string{
		"mysql_image": "mariadb:10.3.8",
	}

	du := deployUploader{
		applicationID: "mysql",
		chID:          chID,
		csMac:         csMac,
		client:        deps,
		resources:     resourceMeta,
		osOpen:        deps.Open,
		osStat:        deps.Stat,
	}
	ids, err := du.upload(passedResourceValues, map[string]int{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ids, gc.DeepEquals, map[string]string{
		"mysql_image": "id-mysql_image",
	})

	expectedUpload := charmresource.Resource{
		Meta:   resourceMeta["mysql_image"],
		Origin: charmresource.OriginUpload,
	}

	expectedUploadData := `
registrypath: mariadb:10.3.8
username: ""
password: ""
`[1:]
	s.stub.CheckCallNames(c, "Open", "Open", "UploadPendingResource")
	s.stub.CheckCall(c, 2, "UploadPendingResource", "mysql", expectedUpload, "mariadb:10.3.8", expectedUploadData)
}

func (s DeploySuite) TestDeployDockerResourceJSONFile(c *gc.C) {
	fileContents := `
{
  "ImageName": "registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image",
  "Username": "docker-registry",
  "Password": "hunter2"
}
`
	dir := c.MkDir()
	jsonFile := path.Join(dir, "details.json")
	err := ioutil.WriteFile(jsonFile, []byte(fileContents), 0600)
	c.Assert(err, jc.ErrorIsNil)
	deps := uploadDeps{s.stub, []byte(fileContents)}
	cURL := charm.MustParseURL("cs:~a-user/mysql-k8s-5")
	chID := charmstore.CharmID{
		URL: cURL,
	}
	csMac := &macaroon.Macaroon{}
	resourceMeta := map[string]charmresource.Meta{
		"mysql_image": {
			Name: "mysql_image",
			Type: charmresource.TypeContainerImage,
		},
	}

	passedResourceValues := map[string]string{
		"mysql_image": jsonFile,
	}
	du := deployUploader{
		applicationID: "mysql",
		chID:          chID,
		csMac:         csMac,
		client:        deps,
		resources:     resourceMeta,
		osOpen:        deps.Open,
		osStat:        deps.Stat,
	}
	ids, err := du.upload(passedResourceValues, map[string]int{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ids, gc.DeepEquals, map[string]string{
		"mysql_image": "id-mysql_image",
	})

	expectedUpload := charmresource.Resource{
		Meta:   resourceMeta["mysql_image"],
		Origin: charmresource.OriginUpload,
	}

	expectedUploadData := `
registrypath: registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image
username: docker-registry
password: hunter2
`[1:]
	s.stub.CheckCallNames(c, "Open", "Open", "UploadPendingResource")
	s.stub.CheckCall(c, 2, "UploadPendingResource", "mysql", expectedUpload, jsonFile, expectedUploadData)
}

func (s DeploySuite) TestDeployDockerResourceYAMLFile(c *gc.C) {
	fileContents := `
registrypath: registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image
username: docker-registry
password: hunter2
`
	dir := c.MkDir()
	jsonFile := path.Join(dir, "details.yaml")
	err := ioutil.WriteFile(jsonFile, []byte(fileContents), 0600)
	c.Assert(err, jc.ErrorIsNil)
	deps := uploadDeps{s.stub, []byte(fileContents)}
	cURL := charm.MustParseURL("cs:~a-user/mysql-k8s-5")
	chID := charmstore.CharmID{
		URL: cURL,
	}
	csMac := &macaroon.Macaroon{}
	resourceMeta := map[string]charmresource.Meta{
		"mysql_image": {
			Name: "mysql_image",
			Type: charmresource.TypeContainerImage,
		},
	}

	passedResourceValues := map[string]string{
		"mysql_image": jsonFile,
	}
	du := deployUploader{
		applicationID: "mysql",
		chID:          chID,
		csMac:         csMac,
		client:        deps,
		resources:     resourceMeta,
		osOpen:        deps.Open,
		osStat:        deps.Stat,
	}
	ids, err := du.upload(passedResourceValues, map[string]int{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ids, gc.DeepEquals, map[string]string{
		"mysql_image": "id-mysql_image",
	})

	expectedUpload := charmresource.Resource{
		Meta:   resourceMeta["mysql_image"],
		Origin: charmresource.OriginUpload,
	}

	expectedUploadData := `
registrypath: registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image
username: docker-registry
password: hunter2
`[1:]
	s.stub.CheckCallNames(c, "Open", "Open", "UploadPendingResource")
	s.stub.CheckCall(c, 2, "UploadPendingResource", "mysql", expectedUpload, jsonFile, expectedUploadData)
}

func (s DeploySuite) TestUnMarshallingDockerDetails(c *gc.C) {
	content := `
registrypath: registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image
username: docker-registry
password: hunter2
`
	data := bytes.NewBufferString(content)
	dets, err := unMarshalDockerDetails(data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dets, gc.DeepEquals, resources.DockerImageDetails{
		RegistryPath: "registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image",
		Username:     "docker-registry",
		Password:     "hunter2",
	})

	content = `
{
"ImageName": "registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image",
"Username": "docker-registry",
"Password": "hunter2"
}
`
	data = bytes.NewBufferString(content)
	dets, err = unMarshalDockerDetails(data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dets, gc.DeepEquals, resources.DockerImageDetails{
		RegistryPath: "registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image",
		Username:     "docker-registry",
		Password:     "hunter2",
	})

	content = `
path: registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image@sha256:516f74
username: docker-registry
password: hunter2
`
	data = bytes.NewBufferString(content)
	_, err = unMarshalDockerDetails(data)
	c.Assert(err, gc.ErrorMatches, "docker image path \"\" not valid")
}

func osOpen(path string) (ReadSeekCloser, error) {
	return os.Open(path)
}

func (s DeploySuite) TestGetDockerDetailsData(c *gc.C) {
	result, err := getDockerDetailsData("registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image", osOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, resources.DockerImageDetails{
		RegistryPath: "registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image",
		Username:     "",
		Password:     "",
	})

	_, err = getDockerDetailsData("/path/doesnt/exist.yaml", osOpen)
	c.Assert(err, gc.ErrorMatches, "filepath or registry path: /path/doesnt/exist.yaml not valid")

	_, err = getDockerDetailsData(".invalid-reg-path", osOpen)
	c.Assert(err, gc.ErrorMatches, "filepath or registry path: .invalid-reg-path not valid")

	dir := c.MkDir()
	yamlFile := path.Join(dir, "actually-yaml-file")
	err = ioutil.WriteFile(yamlFile, []byte("registrypath: mariadb/mariadb:10.2"), 0600)
	c.Assert(err, jc.ErrorIsNil)
	result, err = getDockerDetailsData(yamlFile, osOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, resources.DockerImageDetails{
		RegistryPath: "mariadb/mariadb:10.2",
		Username:     "",
		Password:     "",
	})
}

type uploadDeps struct {
	stub *testing.Stub
	data []byte
}

func (s uploadDeps) AddPendingResources(applicationID string, charmID charmstore.CharmID, csMac *macaroon.Macaroon, resources []charmresource.Resource) (ids []string, err error) {
	charmresource.Sort(resources)
	s.stub.AddCall("AddPendingResources", applicationID, charmID, csMac, resources)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	ids = make([]string, len(resources))
	for i, res := range resources {
		ids[i] = "id-" + res.Name
	}
	return ids, nil
}

func (s uploadDeps) UploadPendingResource(applicationID string, resource charmresource.Resource, filename string, r io.ReadSeeker) (id string, err error) {
	data := new(bytes.Buffer)

	// we care the right data has been passed, not the right io.ReaderSeeker pointer.
	_, err = data.ReadFrom(r)
	if err != nil {
		return "", err
	}
	s.stub.AddCall("UploadPendingResource", applicationID, resource, filename, data.String())
	if err := s.stub.NextErr(); err != nil {
		return "", err
	}
	return "id-" + resource.Name, nil
}

func (s uploadDeps) Open(name string) (ReadSeekCloser, error) {
	s.stub.AddCall("Open", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	if err := resources.ValidateDockerRegistryPath(name); err == nil && !strings.HasSuffix(name, ".txt") {
		return nil, errors.New("invalid file")
	}
	return rsc{bytes.NewBuffer(s.data)}, nil
}

func (s uploadDeps) Stat(name string) error {
	s.stub.AddCall("Stat", name)
	return s.stub.NextErr()
}

type rsc struct {
	*bytes.Buffer
}

func (r rsc) Close() error {
	return nil
}
func (rsc) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}
