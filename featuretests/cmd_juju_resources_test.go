// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/juju/resource"
	coretesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type ResourcesCmdSuite struct {
	coretesting.JujuConnSuite

	appOne *state.Application

	charmName   string
	appOneName  string
	unitOneName string

	client *stubCharmStore
}

func (s *ResourcesCmdSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.charmName = "starsay"
	s.appOneName = "app1"
	charmOne := s.AddTestingCharm(c, s.charmName)

	s.appOne = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  s.appOneName,
		Charm: charmOne,
	})
	unitOne := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.appOne,
		SetCharmURL: true,
	})
	s.unitOneName = unitOne.Name()

	s.client = &stubCharmStore{
		stub: &testing.Stub{},
		listResources: func() [][]charmresource.Resource {
			metas := charmOne.Meta().Resources
			rs := []charmresource.Resource{}
			for n, meta := range metas {
				rs = append(rs, charmRes(c, n, meta))
			}
			return [][]charmresource.Resource{rs}
		},
	}

}

// This test only verifies that component-based resources commands don't panic.
func (s *ResourcesCmdSuite) TestResourcesCommands(c *gc.C) {
	// check "juju charm-resources..."
	s.runCharmResourcesCommand(c)

	// check "juju resources <application>"
	context, err := runCommand(c, "resources", s.appOneName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(context), jc.Contains, `
Resource          Supplied by  Revision
install-resource  upload       -
store-resource    upload       -
upload-resource   upload       -

`[1:])

	// check "juju resources <unit>"
	context, err = runCommand(c, "resources", s.unitOneName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
Resource          Revision
install-resource  -
store-resource    -
upload-resource   -

`[1:])

	// check "juju attach-resource"
	context, err = runCommand(c, "attach-resource", s.appOneName, "install-resource=oops")
	c.Assert(err, gc.ErrorMatches, "cmd: error out silently")
	c.Assert(cmdtesting.Stderr(context), jc.Contains, `ERROR failed to upload resource "install-resource": open oops: no such file or directory`)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "")

	// Empty files are fine.
	filename := filepath.Join(c.MkDir(), "empty.txt")
	err = ioutil.WriteFile(filename, []byte{}, 0755)
	c.Assert(err, jc.ErrorIsNil)
	_, err = runCommand(c, "attach-resource", s.appOneName, "install-resource="+filename)
	c.Check(err, jc.ErrorIsNil)
}

func (s *ResourcesCmdSuite) runCharmResourcesCommand(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, resource.NewCharmResourcesCommand(s.client), s.charmName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
Resource          Revision
install-resource  1
store-resource    1
upload-resource   1

`[1:])
	s.client.stub.CheckCallNames(c, "ListResources")
}

type stubCharmStore struct {
	stub *testing.Stub

	listResources func() [][]charmresource.Resource
}

func (s *stubCharmStore) ListResources(charms []charmstore.CharmID) ([][]charmresource.Resource, error) {
	s.stub.AddCall("ListResources", charms)
	return s.listResources(), s.stub.NextErr()
}

func charmRes(c *gc.C, name string, meta charmresource.Meta) charmresource.Resource {
	content := name
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)

	res := charmresource.Resource{
		Meta:        meta,
		Origin:      charmresource.OriginStore,
		Revision:    1,
		Fingerprint: fp,
		Size:        int64(len(content)),
	}
	err = res.Validate()
	c.Assert(err, jc.ErrorIsNil)
	return res
}
