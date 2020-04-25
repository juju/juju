// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"time" // Only using time func.

	charmresource "github.com/juju/charm/v7/resource"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/component/all"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourcetesting"
	"github.com/juju/juju/testing"
)

func init() {
	if err := all.RegisterForServer(); err != nil {
		panic(err)
	}
}

var _ = gc.Suite(&ResourcesSuite{})

type ResourcesSuite struct {
	ConnSuite
}

func (s *ResourcesSuite) TestFunctional(c *gc.C) {
	ch := s.ConnSuite.AddTestingCharm(c, "wordpress")
	s.ConnSuite.AddTestingApplication(c, "a-application", ch)

	st, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)

	resources, err := st.ListResources("a-application")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(resources.Resources, gc.HasLen, 0)

	data := "spamspamspam"
	res := newResource(c, "spam", data)
	file := bytes.NewBufferString(data)

	_, err = st.SetResource("a-application", res.Username, res.Resource, file)
	c.Assert(err, jc.ErrorIsNil)

	csResources := []charmresource.Resource{res.Resource}
	err = st.SetCharmStoreResources("a-application", csResources, testing.NonZeroTime())
	c.Assert(err, jc.ErrorIsNil)

	resources, err = st.ListResources("a-application")
	c.Assert(err, jc.ErrorIsNil)

	res.Timestamp = resources.Resources[0].Timestamp
	c.Check(resources, jc.DeepEquals, resource.ApplicationResources{
		Resources:           []resource.Resource{res},
		CharmStoreResources: csResources,
	})

	// TODO(ericsnow) Add more as state.Resources grows more functionality.
}

func newResource(c *gc.C, name, data string) resource.Resource {
	opened := resourcetesting.NewResource(c, nil, name, "a-application", data)
	res := opened.Resource
	res.Timestamp = time.Unix(res.Timestamp.Unix(), 0)
	return res
}
