// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"encoding/json"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

type StatusSuite struct{}

var _ = gc.Suite(&StatusSuite{})

func (s *StatusSuite) TestMarshallApplicationStatusCharmVersion(c *gc.C) {
	as := params.ApplicationStatus{
		CharmVersion: "666",
	}
	data, err := json.Marshal(as)
	c.Assert(err, jc.ErrorIsNil)

	var m map[string]interface{}
	err = json.Unmarshal(data, &m)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(m["charm-verion"], gc.Equals, "666")
	c.Assert(m["charm-version"], gc.Equals, "666")
}
