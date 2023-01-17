// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"bytes"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/juju/charm/v10"
	charmresource "github.com/juju/charm/v10/resource"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/testcharms"
)

type RefreshResourceSuite struct {
	RepoSuiteBaseSuite
}

var _ = gc.Suite(&RefreshResourceSuite{})

func (s *RefreshResourceSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "darwin" {
		c.Skip("Mongo failures on macOS")
	}
	s.RepoSuiteBaseSuite.SetUpTest(c)

	// TODO: remove this patch once we removed all the old series from tests in current package.
	s.PatchValue(&deployer.SupportedJujuSeries,
		func(time.Time, string, string) (set.Strings, error) {
			return set.NewStrings(
				"centos7", "centos8", "centos9", "genericlinux", "kubernetes", "opensuseleap",
				"jammy", "focal", "bionic", "xenial", "quantal",
			), nil
		},
	)

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

func (s *RefreshResourceSuite) TestUpgradeWithResources(c *gc.C) {
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
	err := os.WriteFile(path.Join(myriakPath.Path, "metadata.yaml"), []byte(riakResourceMeta), 0644)
	c.Assert(err, jc.ErrorIsNil)

	data := []byte("some-data")
	fp, err := charmresource.GenerateFingerprint(bytes.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)

	resourceFile := path.Join(c.MkDir(), "data.lib")
	err = os.WriteFile(resourceFile, data, 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, NewRefreshCommand(),
		"riak", "--path="+myriakPath.Path, "--resource", "data="+resourceFile)
	c.Assert(err, jc.ErrorIsNil)

	resources := s.State.Resources()

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

func resourceHash(content string) charmresource.Fingerprint {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	if err != nil {
		panic(err)
	}
	return fp
}
