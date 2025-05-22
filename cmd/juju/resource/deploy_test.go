// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"bytes"
	"context"
	"os"
	"path"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	apiresources "github.com/juju/juju/api/client/resources"
	"github.com/juju/juju/cmd/modelcmd"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/testhelpers"
)

type DeploySuite struct {
	testhelpers.IsolationSuite

	stub *testhelpers.Stub
}

func TestDeploySuite(t *stdtesting.T) {
	tc.Run(t, &DeploySuite{})
}

func (s *DeploySuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testhelpers.Stub{}
}

func (s *DeploySuite) TestDeployResourcesWithoutFiles(c *tc.C) {
	deps := uploadDeps{stub: s.stub}
	cURL := "spam"
	chID := apiresources.CharmID{
		URL: cURL,
	}
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

	ids, err := DeployResources(c.Context(), DeployResourcesArgs{
		ApplicationID:  "mysql",
		CharmID:        chID,
		ResourceValues: nil,
		Client:         deps,
		ResourcesMeta:  resources,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(ids, tc.DeepEquals, map[string]string{
		"store-tarball": "id-store-tarball",
		"store-zip":     "id-store-zip",
	})

	s.stub.CheckCallNames(c, "AddPendingResources")
	s.stub.CheckCall(c, 0, "AddPendingResources", "mysql", chID, []charmresource.Resource{{
		Meta:     resources["store-tarball"],
		Origin:   charmresource.OriginStore,
		Revision: -1,
	}, {
		Meta:     resources["store-zip"],
		Origin:   charmresource.OriginStore,
		Revision: -1,
	}})
}

func (s *DeploySuite) TestUploadFilesOnly(c *tc.C) {
	deps := uploadDeps{stub: s.stub, data: []byte("file contents")}
	cURL := "spam"
	chID := apiresources.CharmID{
		URL: cURL,
	}
	du := deployUploader{
		applicationID: "mysql",
		chID:          chID,
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
		filesystem: deps,
	}

	files := map[string]string{
		"upload": "foobar.txt",
	}
	revisions := map[string]int{}
	ids, err := du.upload(c.Context(), files, revisions)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ids, tc.DeepEquals, map[string]string{
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
	s.stub.CheckCall(c, 1, "AddPendingResources", "mysql", chID, expectedStore)
	s.stub.CheckCall(c, 2, "Open", "foobar.txt")

	expectedUploadArgs := apiresources.UploadPendingResourceArgs{
		ApplicationID: "mysql",
		CharmID:       chID,
		Resource: charmresource.Resource{
			Meta:   du.resources["upload"],
			Origin: charmresource.OriginUpload,
		},
		Filename: "foobar.txt",
	}
	s.stub.CheckCall(c, 3, "UploadPendingResource", expectedUploadArgs)
}

func (s *DeploySuite) TestUploadRevisionsOnly(c *tc.C) {
	deps := uploadDeps{stub: s.stub}
	cURL := "spam"
	chID := apiresources.CharmID{
		URL: cURL,
	}
	du := deployUploader{
		applicationID: "mysql",
		chID:          chID,
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
		filesystem: deps,
	}

	files := map[string]string{}
	revisions := map[string]int{
		"store": 3,
	}
	ids, err := du.upload(c.Context(), files, revisions)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ids, tc.DeepEquals, map[string]string{
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
	s.stub.CheckCall(c, 0, "AddPendingResources", "mysql", chID, expectedStore)
}

func (s *DeploySuite) TestUploadFilesAndRevisions(c *tc.C) {
	deps := uploadDeps{stub: s.stub, data: []byte("file contents")}
	cURL := "spam"
	chID := apiresources.CharmID{
		URL: cURL,
	}
	du := deployUploader{
		applicationID: "mysql",
		chID:          chID,
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
		filesystem: deps,
	}

	files := map[string]string{
		"upload": "foobar.txt",
	}
	revisions := map[string]int{
		"store": 3,
	}
	ids, err := du.upload(c.Context(), files, revisions)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ids, tc.DeepEquals, map[string]string{
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
	s.stub.CheckCall(c, 1, "AddPendingResources", "mysql", chID, expectedStore)
	s.stub.CheckCall(c, 2, "Open", "foobar.txt")

	expectedUploadArgs := apiresources.UploadPendingResourceArgs{
		ApplicationID: "mysql",
		CharmID:       chID,
		Resource: charmresource.Resource{
			Meta:   du.resources["upload"],
			Origin: charmresource.OriginUpload,
		},
		Filename: "foobar.txt",
	}
	s.stub.CheckCall(c, 3, "UploadPendingResource", expectedUploadArgs)
}

func (s *DeploySuite) TestUploadUnexpectedResourceFile(c *tc.C) {
	deps := uploadDeps{stub: s.stub}
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
		filesystem: deps,
	}

	files := map[string]string{"some bad resource": "foobar.txt"}
	revisions := map[string]int{}
	_, err := du.upload(c.Context(), files, revisions)
	c.Check(err, tc.ErrorMatches, `unrecognized resource "some bad resource"`)

	s.stub.CheckNoCalls(c)
}

func (s *DeploySuite) TestUploadUnexpectedResourceRevision(c *tc.C) {
	deps := uploadDeps{stub: s.stub}
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
		filesystem: deps,
	}

	files := map[string]string{}
	revisions := map[string]int{"some bad resource": 2}
	_, err := du.upload(c.Context(), files, revisions)
	c.Check(err, tc.ErrorMatches, `unrecognized resource "some bad resource"`)

	s.stub.CheckNoCalls(c)
}

func (s *DeploySuite) TestMissingResource(c *tc.C) {
	deps := uploadDeps{stub: s.stub}
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
		filesystem: deps,
	}

	// set the error that will be returned by os.Stat
	s.stub.SetErrors(os.ErrNotExist)

	files := map[string]string{"res1": "foobar.txt"}
	revisions := map[string]int{}
	_, err := du.upload(c.Context(), files, revisions)
	c.Check(err, tc.ErrorMatches, `file for resource "res1".*`)
	c.Check(errors.Cause(err), tc.Satisfies, os.IsNotExist)
}

func (s *DeploySuite) TestDeployDockerResource(c *tc.C) {
	tests := []struct {
		about              string
		fileContents       string
		resourceValue      string
		expectedUploadData string
		uploadError        string
	}{
		{
			about:         "registry path string",
			resourceValue: "mariadb:10.3.8",
			expectedUploadData: `
registrypath: mariadb:10.3.8
username: ""
password: ""
`[1:],
		},
		{
			about: "resource json file",
			fileContents: `
{
  "ImageName": "registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image",
  "Username": "docker-registry",
  "Password": "hunter2"
}
`,
			expectedUploadData: `
registrypath: registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image
username: docker-registry
password: hunter2
`[1:],
		},
		{
			about: "resource yaml file",
			fileContents: `
registrypath: registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image
username: docker-registry
password: hunter2
`,
			expectedUploadData: `
registrypath: registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image
username: docker-registry
password: hunter2
`[1:],
		},
		{
			about: "invalid json file",
			fileContents: `
{
  "ImageName": "registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image",
  "Username": "docker-registry",,
  "Password": "hunter2"
}
`,
			uploadError: ".*json parsing.*",
		},
		{
			about: "invalid yaml file",
			fileContents: `
registrypath: registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image
username: docker-registry
password: 'hunter2',,
`,
			uploadError: ".*yaml.*",
		},
		{
			about:        "providing binary file (e.g. OCI image)",
			fileContents: "\x1f\x8b\x00\xba\xb3\x85\x00", // just some random binary data
			uploadError:  ".*expected json or yaml file.*",
		},
	}

	for i, t := range tests {
		c.Logf("test %d: %s", i, t.about)
		// Redo test setup between runs
		s.SetUpTest(c)
		deps := uploadDeps{stub: s.stub}
		resourceValue := t.resourceValue

		if t.fileContents != "" {
			dir := c.MkDir()
			resourceValue = path.Join(dir, "details.json")
			err := os.WriteFile(resourceValue, []byte(t.fileContents), 0600)
			c.Assert(err, tc.ErrorIsNil)
			deps.data = []byte(t.fileContents)
		}

		cURL := "mysql-k8s"
		chID := apiresources.CharmID{
			URL: cURL,
		}

		resourceMeta := map[string]charmresource.Meta{
			"mysql_image": {
				Name: "mysql_image",
				Type: charmresource.TypeContainerImage,
			},
		}

		passedResourceValues := map[string]string{
			"mysql_image": resourceValue,
		}

		du := deployUploader{
			applicationID: "mysql",
			chID:          chID,
			client:        deps,
			resources:     resourceMeta,
			filesystem:    deps,
		}
		ids, err := du.upload(c.Context(), passedResourceValues, map[string]int{})
		if t.uploadError != "" {
			c.Assert(err, tc.ErrorMatches, t.uploadError)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Check(ids, tc.DeepEquals, map[string]string{
			"mysql_image": "id-mysql_image",
		})

		expectedUploadArgs := apiresources.UploadPendingResourceArgs{
			ApplicationID: "mysql",
			CharmID:       chID,
			Resource: charmresource.Resource{
				Meta:   resourceMeta["mysql_image"],
				Origin: charmresource.OriginUpload,
			},
			Filename: resourceValue,
		}

		s.stub.CheckCallNames(c, "Open", "Open", "UploadPendingResource")
		s.stub.CheckCall(c, 2, "UploadPendingResource", expectedUploadArgs)
	}
}

func (s *DeploySuite) TestUnMarshallingDockerDetails(c *tc.C) {
	content := `
registrypath: registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image
username: docker-registry
password: hunter2
`
	data := bytes.NewBufferString(content)
	dets, err := unMarshalDockerDetails(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dets, tc.DeepEquals, docker.DockerImageDetails{
		RegistryPath: "registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "docker-registry",
				Password: "hunter2",
			},
		},
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dets, tc.DeepEquals, docker.DockerImageDetails{
		RegistryPath: "registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "docker-registry",
				Password: "hunter2",
			},
		},
	})

	content = `
path: registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image@sha256:516f74
username: docker-registry
password: hunter2
`
	data = bytes.NewBufferString(content)
	_, err = unMarshalDockerDetails(data)
	c.Assert(err, tc.ErrorMatches, "docker image path \"\" not valid")
}

type osFilesystem struct {
	modelcmd.Filesystem
}

func (osFilesystem) Open(name string) (modelcmd.ReadSeekCloser, error) {
	return os.Open(name)
}

func (s *DeploySuite) TestGetDockerDetailsData(c *tc.C) {
	fs := osFilesystem{}
	result, err := getDockerDetailsData("registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image", fs.Open)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, docker.DockerImageDetails{
		RegistryPath: "registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "",
				Password: "",
			},
		},
	})

	_, err = getDockerDetailsData("/path/doesnt/exist.yaml", fs.Open)
	c.Assert(err, tc.ErrorMatches, "filepath or registry path: /path/doesnt/exist.yaml not valid")

	_, err = getDockerDetailsData(".invalid-reg-path", fs.Open)
	c.Assert(err, tc.ErrorMatches, "filepath or registry path: .invalid-reg-path not valid")

	dir := c.MkDir()
	yamlFile := path.Join(dir, "actually-yaml-file")
	err = os.WriteFile(yamlFile, []byte("registrypath: mariadb/mariadb:10.2"), 0600)
	c.Assert(err, tc.ErrorIsNil)
	result, err = getDockerDetailsData(yamlFile, fs.Open)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, docker.DockerImageDetails{
		RegistryPath: "mariadb/mariadb:10.2",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "",
				Password: "",
			},
		},
	})
}

type uploadDeps struct {
	modelcmd.Filesystem
	stub *testhelpers.Stub
	data []byte
}

func (s uploadDeps) AddPendingResources(ctx context.Context, applicationID string, charmID apiresources.CharmID, resources []charmresource.Resource) (ids []string, err error) {
	charmresource.Sort(resources)
	s.stub.AddCall("AddPendingResources", applicationID, charmID, resources)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	ids = make([]string, len(resources))
	for i, res := range resources {
		ids[i] = "id-" + res.Name
	}
	return ids, nil
}

func (s uploadDeps) UploadPendingResource(_ context.Context, args apiresources.UploadPendingResourceArgs) (id string, err error) {
	data := new(bytes.Buffer)

	// we care the right data has been passed, not the right io.ReaderSeeker pointer.
	_, err = data.ReadFrom(args.Reader)
	if err != nil {
		return "", err
	}
	args.Reader = nil
	s.stub.AddCall("UploadPendingResource", args)
	if err := s.stub.NextErr(); err != nil {
		return "", err
	}
	return "id-" + args.Resource.Name, nil
}

func (s uploadDeps) Open(name string) (modelcmd.ReadSeekCloser, error) {
	s.stub.AddCall("Open", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	if err := docker.ValidateDockerRegistryPath(name); err == nil && !strings.HasSuffix(name, ".txt") {
		return nil, errors.New("invalid file")
	}
	return rsc{bytes.NewBuffer(s.data)}, nil
}

func (s uploadDeps) Stat(name string) (os.FileInfo, error) {
	s.stub.AddCall("Stat", name)
	return nil, s.stub.NextErr()
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
