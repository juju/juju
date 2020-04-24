// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"bytes"
	"io/ioutil"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/juju/charm/v7"
	charmresource "github.com/juju/charm/v7/resource"
	csclientparams "github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/juju/api/charms"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	jjcharmstore "github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/testcharms"
)

type UpgradeCharmResourceSuite struct {
	RepoSuiteBaseSuite
}

var _ = gc.Suite(&UpgradeCharmResourceSuite{})

func (s *UpgradeCharmResourceSuite) SetUpTest(c *gc.C) {
	s.RepoSuiteBaseSuite.SetUpTest(c)
	chPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "riak")
	err := runDeploy(c, chPath, "riak", "--series", "quantal", "--force")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:quantal/riak-7")
	riak, _ := s.RepoSuite.AssertApplication(c, "riak", curl, 1, 1)
	c.Assert(err, jc.ErrorIsNil)
	_, forced, err := riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
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

	myriakPath := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "riak")
	err := ioutil.WriteFile(path.Join(myriakPath.Path, "metadata.yaml"), []byte(riakResourceMeta), 0644)
	c.Assert(err, jc.ErrorIsNil)

	data := []byte("some-data")
	fp, err := charmresource.GenerateFingerprint(bytes.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)

	resourceFile := path.Join(c.MkDir(), "data.lib")
	err = ioutil.WriteFile(resourceFile, data, 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, NewUpgradeCharmCommand(),
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

type UpgradeCharmStoreResourceSuite struct {
	FakeStoreStateSuite
}

var _ = gc.Suite(&UpgradeCharmStoreResourceSuite{})

func (s *UpgradeCharmStoreResourceSuite) TestDeployStarsaySuccess(c *gc.C) {
	ch := s.setupCharm(c, "bionic/starsay-1", "starsay", "bionic")

	// let's make a fake resource file to upload
	resourceContent := "some-data"

	resourceFile := path.Join(c.MkDir(), "data.xml")
	err := ioutil.WriteFile(resourceFile, []byte(resourceContent), 0644)
	c.Assert(err, jc.ErrorIsNil)

	deploy := newDeployCommand()
	deploy.DeployResources = func(applicationID string,
		chID jjcharmstore.CharmID,
		csMac *macaroon.Macaroon,
		filesAndRevisions map[string]string,
		resources map[string]charmresource.Meta,
		conn base.APICallCloser,
	) (ids map[string]string, err error) {
		return deployResources(s.State, applicationID, resources)
	}
	deploy.NewCharmRepo = func() (*charmStoreAdaptor, error) {
		return s.fakeAPI.charmStoreAdaptor, nil
	}

	_, output, err := runDeployWithOutput(c, modelcmd.Wrap(deploy), "bionic/starsay", "--resource", "upload-resource="+resourceFile)
	c.Assert(err, jc.ErrorIsNil)

	expectedOutput := `Located charm "cs:bionic/starsay-1".
Deploying charm "cs:bionic/starsay-1".`
	c.Assert(output, gc.Equals, expectedOutput)
	s.assertCharmsUploaded(c, "cs:bionic/starsay-1")
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"starsay": {charm: "cs:bionic/starsay-1", config: ch.Config().DefaultSettings()},
	})

	unit, err := s.State.Unit("starsay/0")
	c.Assert(err, jc.ErrorIsNil)
	tags := []names.UnitTag{unit.UnitTag()}
	errs, err := s.APIState.UnitAssigner().AssignUnits(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []error{nil})

	res, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	appResources, err := res.ListResources("starsay")
	c.Assert(err, jc.ErrorIsNil)

	sort.Sort(byname(appResources.Resources))

	c.Assert(appResources.Resources, gc.HasLen, 3)
	c.Check(appResources.Resources[2].Timestamp, gc.Not(gc.Equals), time.Time{})
	appResources.Resources[2].Timestamp = time.Time{}

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

	c.Check(appResources.Resources, jc.DeepEquals, expectedResources)

	oldCharmStoreResources := make([]charmresource.Resource, len(appResources.CharmStoreResources))
	copy(oldCharmStoreResources, appResources.CharmStoreResources)

	sort.Sort(csbyname(oldCharmStoreResources))

	s.setupCharm(c, "bionic/starsay-2", "starsay", "bionic")
	charmClient := &mockCharmClient{
		charmInfo: &charms.CharmInfo{
			URL:  "bionic/starsay-2",
			Meta: &charm.Meta{},
		},
	}
	charmAdder := &mockCharmAdder{}
	upgrade := NewUpgradeCharmCommandForStateTest(
		func(
			bakeryClient *httpbakery.Client,
			csURL string,
			channel csclientparams.Channel,
		) charmrepoForDeploy {
			return s.fakeAPI
		},
		func(conn api.Connection) CharmAdder {
			return charmAdder
		},
		func(conn base.APICallCloser) CharmClient {
			return charmClient
		},
		func(applicationID string,
			chID jjcharmstore.CharmID,
			csMac *macaroon.Macaroon,
			filesAndRevisions map[string]string,
			resources map[string]charmresource.Meta,
			conn base.APICallCloser,
		) (ids map[string]string, err error) {
			return deployResources(s.State, applicationID, resources)
		},
		func(conn base.APICallCloser) CharmAPIClient {
			return &mockCharmAPIClient{
				charmURL: charm.MustParseURL("bionic/starsay-1"),
			}
		},
	)

	_, err = cmdtesting.RunCommand(c, upgrade, "starsay")
	c.Assert(err, jc.ErrorIsNil)

	charmAdder.CheckCall(c, 0,
		"AddCharm", charm.MustParseURL("cs:bionic/starsay-2"), csclientparams.NoChannel, false)

	res, err = s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	appResources, err = res.ListResources("starsay")
	c.Assert(err, jc.ErrorIsNil)

	sort.Sort(byname(appResources.Resources))

	c.Assert(appResources.Resources, gc.HasLen, 3)
	c.Check(appResources.Resources[2].Timestamp, gc.Not(gc.Equals), time.Time{})
	appResources.Resources[2].Timestamp = time.Time{}

	// ensure that we haven't overridden the previously uploaded resource.
	c.Check(appResources.Resources, jc.DeepEquals, expectedResources)

	sort.Sort(csbyname(appResources.CharmStoreResources))
	c.Check(oldCharmStoreResources, gc.DeepEquals, appResources.CharmStoreResources)
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
