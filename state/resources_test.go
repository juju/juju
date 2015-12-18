// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/component/all"
	"github.com/juju/juju/resource"
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
	st, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)

	resources, err := st.ListResources("a-service")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(resources, gc.HasLen, 0)

	data := "spamspamspam"
	res := newResource(c, "spam", data)
	file := bytes.NewBufferString(data)

	err = st.SetResource("a-service", res, file)
	c.Assert(err, jc.ErrorIsNil)

	resources, err = st.ListResources("a-service")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(resources, jc.DeepEquals, []resource.Resource{
		res,
	})

	// TODO(ericsnow) Add more as state.Resources grows more functionality.
}

func newResource(c *gc.C, name, data string) resource.Resource {
	fp, err := charmresource.GenerateFingerprint([]byte(data))
	c.Assert(err, jc.ErrorIsNil)

	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:    name,
				Type:    charmresource.TypeFile,
				Path:    name + ".tgz",
				Comment: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        int64(len(data)),
		},
		Username:  "a-user",
		Timestamp: time.Now(),
	}
	err = res.Validate()
	c.Assert(err, jc.ErrorIsNil)
	return res
}
