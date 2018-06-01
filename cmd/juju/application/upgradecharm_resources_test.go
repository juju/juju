// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"bytes"
	"io/ioutil"
	"net/http/httptest"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	charmresource "gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/charmrepo.v3/csclient"
	"gopkg.in/juju/charmstore.v5"

	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/component/all"
	"github.com/juju/juju/constraints"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

type UpgradeCharmResourceSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&UpgradeCharmResourceSuite{})

func (s *UpgradeCharmResourceSuite) SetUpSuite(c *gc.C) {
	s.RepoSuite.SetUpSuite(c)
	all.RegisterForServer()
}

func (s *UpgradeCharmResourceSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	chPath := testcharms.Repo.ClonedDirPath(s.CharmsPath, "riak")
	_, err := runDeploy(c, chPath, "riak", "--series", "quantal", "--force")
	c.Assert(err, jc.ErrorIsNil)
	riak, err := s.State.Application("riak")
	c.Assert(err, jc.ErrorIsNil)
	ch, forced, err := riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 7)
	c.Assert(forced, jc.IsFalse)
}

func (s *UpgradeCharmResourceSuite) TestUpgradeWithResources(c *gc.C) {
	const riakResourceMeta = `
name: riak
summary: "K/V storage engine"
description: "Scalable K/V Store in Erlang with Clocks :-)"
provides:
  endpoint:
    interface: http
  admin:
    interface: http
peers:
  ring:
    interface: riak
resources:
  data:
    type: file
    filename: foo.lib
    description: some comment
`

	myriakPath := testcharms.Repo.ClonedDir(c.MkDir(), "riak")
	err := ioutil.WriteFile(path.Join(myriakPath.Path, "metadata.yaml"), []byte(riakResourceMeta), 0644)
	c.Assert(err, jc.ErrorIsNil)

	data := []byte("some-data")
	fp, err := charmresource.GenerateFingerprint(bytes.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)

	resourceFile := path.Join(c.MkDir(), "data.lib")
	err = ioutil.WriteFile(resourceFile, data, 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, application.NewUpgradeCharmCommand(),
		"riak", "--path="+myriakPath.Path, "--resource", "data="+resourceFile)
	c.Assert(err, jc.ErrorIsNil)

	resources, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)

	sr, err := resources.ListResources("riak")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(sr.Resources, gc.HasLen, 1)

	c.Check(sr.Resources[0].ApplicationID, gc.Equals, "riak")

	// Most of this is just a sanity check... this is all tested elsewhere.
	c.Check(sr.Resources[0].PendingID, gc.Equals, "")
	c.Check(sr.Resources[0].Username, gc.Not(gc.Equals), "")
	c.Check(sr.Resources[0].ID, gc.Not(gc.Equals), "")
	c.Check(sr.Resources[0].Timestamp.IsZero(), jc.IsFalse)

	// Ensure we get the data we passed in from the metadata.yaml.
	c.Check(sr.Resources[0].Resource, gc.DeepEquals, charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        "data",
			Type:        charmresource.TypeFile,
			Path:        "foo.lib",
			Description: "some comment",
		},
		Origin:      charmresource.OriginUpload,
		Fingerprint: fp,
		Size:        int64(len(data)),
	})
}

// charmStoreSuite is a suite fixture that puts the machinery in
// place to allow testing code that calls addCharmViaAPI.
type charmStoreSuite struct {
	jujutesting.JujuConnSuite
	handler charmstore.HTTPCloseHandler
	srv     *httptest.Server
	client  *csclient.Client
}

func (s *charmStoreSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Set up the charm store testing server.
	db := s.Session.DB("juju-testing")
	params := charmstore.ServerParams{
		AuthUsername: "test-user",
		AuthPassword: "test-password",
	}
	handler, err := charmstore.NewServer(db, nil, "", params, charmstore.V5)
	c.Assert(err, jc.ErrorIsNil)
	s.handler = handler
	s.srv = httptest.NewServer(handler)
	s.client = csclient.New(csclient.Params{
		URL:      s.srv.URL,
		User:     params.AuthUsername,
		Password: params.AuthPassword,
	})

	application.PatchNewCharmStoreClient(s, s.srv.URL)

	// Initialize the charm cache dir.
	s.PatchValue(&charmrepo.CacheDir, c.MkDir())

	// Point the CLI to the charm store testing server.

	// Point the Juju API server to the charm store testing server.
	s.PatchValue(&csclient.ServerURL, s.srv.URL)
}

func (s *charmStoreSuite) TearDownTest(c *gc.C) {
	s.handler.Close()
	s.srv.Close()
	s.JujuConnSuite.TearDownTest(c)
}

type UpgradeCharmStoreResourceSuite struct {
	charmStoreSuite
}

var _ = gc.Suite(&UpgradeCharmStoreResourceSuite{})

func (s *UpgradeCharmStoreResourceSuite) SetUpSuite(c *gc.C) {
	s.charmStoreSuite.SetUpSuite(c)
	err := all.RegisterForServer()
	c.Assert(err, jc.ErrorIsNil)
	err = all.RegisterForClient()
	c.Assert(err, jc.ErrorIsNil)
}

// TODO(ericsnow) Adapt this test to check passing revisions once the
// charmstore endpoints are implemented.

func (s *UpgradeCharmStoreResourceSuite) TestDeployStarsaySuccess(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "trusty/starsay-1", "starsay")

	// let's make a fake resource file to upload
	resourceContent := "some-data"

	resourceFile := path.Join(c.MkDir(), "data.xml")
	err := ioutil.WriteFile(resourceFile, []byte(resourceContent), 0644)
	c.Assert(err, jc.ErrorIsNil)

	output, err := runDeploy(c, "trusty/starsay", "--resource", "upload-resource="+resourceFile)
	c.Assert(err, jc.ErrorIsNil)

	expectedOutput := `Located charm "cs:trusty/starsay-1".
Deploying charm "cs:trusty/starsay-1".`
	c.Assert(output, gc.Equals, expectedOutput)
	s.assertCharmsUploaded(c, "cs:trusty/starsay-1")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"starsay": {charm: "cs:trusty/starsay-1"},
	})
	_, err = s.State.Unit("starsay/0")
	c.Assert(err, jc.ErrorIsNil)

	res, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	svcres, err := res.ListResources("starsay")
	c.Assert(err, jc.ErrorIsNil)

	sort.Sort(byname(svcres.Resources))

	c.Assert(svcres.Resources, gc.HasLen, 3)
	c.Check(svcres.Resources[2].Timestamp, gc.Not(gc.Equals), time.Time{})
	svcres.Resources[2].Timestamp = time.Time{}

	// Note that all charm resources were uploaded by testcharms.UploadCharm
	// so that the charm could be published.
	expectedResources := []resource.Resource{{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "install-resource",
				Type:        charmresource.TypeFile,
				Path:        "gotta-have-it.txt",
				Description: "get things started",
			},
			Origin:      charmresource.OriginStore,
			Revision:    0,
			Fingerprint: resourceHash("install-resource content"),
			Size:        int64(len("install-resource content")),
		},
		ID:            "starsay/install-resource",
		ApplicationID: "starsay",
	}, {
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "store-resource",
				Type:        charmresource.TypeFile,
				Path:        "filename.tgz",
				Description: "One line that is useful when operators need to push it.",
			},
			Origin:      charmresource.OriginStore,
			Revision:    0,
			Fingerprint: resourceHash("store-resource content"),
			Size:        int64(len("store-resource content")),
		},
		ID:            "starsay/store-resource",
		ApplicationID: "starsay",
	}, {
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "upload-resource",
				Type:        charmresource.TypeFile,
				Path:        "somename.xml",
				Description: "Who uses xml anymore?",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    0,
			Fingerprint: resourceHash(resourceContent),
			Size:        int64(len(resourceContent)),
		},
		ID:            "starsay/upload-resource",
		ApplicationID: "starsay",
		Username:      "admin",
		// Timestamp is checked above
	}}

	c.Check(svcres.Resources, jc.DeepEquals, expectedResources)

	oldCharmStoreResources := make([]charmresource.Resource, len(svcres.CharmStoreResources))
	copy(oldCharmStoreResources, svcres.CharmStoreResources)

	sort.Sort(csbyname(oldCharmStoreResources))

	testcharms.UploadCharm(c, s.client, "trusty/starsay-2", "starsay")

	_, err = cmdtesting.RunCommand(c, application.NewUpgradeCharmCommand(), "starsay")
	c.Assert(err, jc.ErrorIsNil)

	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"starsay": {charm: "cs:trusty/starsay-2"},
	})

	res, err = s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	svcres, err = res.ListResources("starsay")
	c.Assert(err, jc.ErrorIsNil)

	sort.Sort(byname(svcres.Resources))

	c.Assert(svcres.Resources, gc.HasLen, 3)
	c.Check(svcres.Resources[2].Timestamp, gc.Not(gc.Equals), time.Time{})
	svcres.Resources[2].Timestamp = time.Time{}

	// ensure that we haven't overridden the previously uploaded resource.
	c.Check(svcres.Resources, jc.DeepEquals, expectedResources)

	sort.Sort(csbyname(svcres.CharmStoreResources))
	c.Check(oldCharmStoreResources, gc.DeepEquals, svcres.CharmStoreResources)
}

func resourceHash(content string) charmresource.Fingerprint {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	if err != nil {
		panic(err)
	}
	return fp
}

type byname []resource.Resource

func (b byname) Len() int           { return len(b) }
func (b byname) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byname) Less(i, j int) bool { return b[i].Name < b[j].Name }

type csbyname []charmresource.Resource

func (b csbyname) Len() int           { return len(b) }
func (b csbyname) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b csbyname) Less(i, j int) bool { return b[i].Name < b[j].Name }

// assertCharmsUploaded checks that the given charm ids have been uploaded.
func (s *charmStoreSuite) assertCharmsUploaded(c *gc.C, ids ...string) {
	charms, err := s.State.AllCharms()
	c.Assert(err, jc.ErrorIsNil)
	uploaded := make([]string, len(charms))
	for i, charm := range charms {
		uploaded[i] = charm.URL().String()
	}
	c.Assert(uploaded, jc.SameContents, ids)
}

// assertApplicationsDeployed checks that the given applications have been deployed.
func (s *charmStoreSuite) assertApplicationsDeployed(c *gc.C, info map[string]applicationInfo) {
	applications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	deployed := make(map[string]applicationInfo, len(applications))
	for _, application := range applications {
		charm, _ := application.CharmURL()
		config, err := application.CharmConfig()
		c.Assert(err, jc.ErrorIsNil)
		if len(config) == 0 {
			config = nil
		}
		constraints, err := application.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		storage, err := application.StorageConstraints()
		c.Assert(err, jc.ErrorIsNil)
		if len(storage) == 0 {
			storage = nil
		}
		deployed[application.Name()] = applicationInfo{
			charm:       charm.String(),
			config:      config,
			constraints: constraints,
			exposed:     application.IsExposed(),
			storage:     storage,
		}
	}
	c.Assert(deployed, jc.DeepEquals, info)
}

// applicationInfo holds information about a deployed application.
type applicationInfo struct {
	charm            string
	config           charm.Settings
	constraints      constraints.Value
	exposed          bool
	storage          map[string]state.StorageConstraints
	endpointBindings map[string]string
}

// runDeploy executes the deploy command in order to deploy the given
// charm or bundle. The deployment stderr output and error are returned.
// TODO(rog) delete this when tests are universally internal or external.
func runDeploy(c *gc.C, args ...string) (string, error) {
	ctx, err := cmdtesting.RunCommand(c, application.NewDeployCommand(), args...)
	return strings.Trim(cmdtesting.Stderr(ctx), "\n"), err
}
