// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"bytes"
	"io/ioutil"
	"path"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/cmd/juju/service"
	"github.com/juju/juju/component/all"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
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
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "riak")

	_, err := testing.RunCommand(c, service.NewDeployCommand(), "local:riak", "riak")
	c.Assert(err, jc.ErrorIsNil)
	riak, err := s.State.Service("riak")
	c.Assert(err, jc.ErrorIsNil)
	ch, forced, err := riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 7)
	c.Assert(forced, jc.IsFalse)
}

var riakResourceMeta = []byte(`
name: riakresource
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
`)

func (s *UpgradeCharmResourceSuite) TestUpgradeWithResources(c *gc.C) {
	myriakPath := testcharms.Repo.ClonedDir(c.MkDir(), "riak")
	err := ioutil.WriteFile(path.Join(myriakPath.Path, "metadata.yaml"), riakResourceMeta, 0644)
	c.Assert(err, jc.ErrorIsNil)

	data := []byte("some-data")
	resourceFile := path.Join(c.MkDir(), "data.lib")
	err = ioutil.WriteFile(resourceFile, data, 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = testing.RunCommand(c, service.NewUpgradeCharmCommand(),
		"riak", "--path="+myriakPath.Path, "--resource", "data="+resourceFile)
	c.Assert(err, jc.ErrorIsNil)

	resources, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)

	sr, err := resources.ListResources("riak")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(sr.Resources, gc.HasLen, 1)

	c.Assert(sr.Resources[0].ServiceID, gc.Equals, "riak")

	// Most of this is just a sanity check... this is all tested elsewhere.
	c.Assert(sr.Resources[0].PendingID, gc.Equals, "")
	c.Assert(sr.Resources[0].Username, gc.Not(gc.Equals), "")
	c.Assert(sr.Resources[0].ID, gc.Not(gc.Equals), "")
	c.Assert(sr.Resources[0].Timestamp.IsZero(), jc.IsFalse)

	fp, err := charmresource.GenerateFingerprint(bytes.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)

	// Ensure we get the data we passed in from the metadata.yaml.
	c.Assert(sr.Resources[0].Resource, gc.DeepEquals, charmresource.Resource{
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
