// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"encoding/json"
	"strings"

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
	c.Assert(string(data), gc.Equals, strings.Replace(`
{"charm-verion":"666","charm":"","series":"","exposed":false,"life":"","relations":null,"can-upgrade-to":"","subordinate-to":null,
"units":null,"meter-statuses":null,"status":{"status":"","info":"","data":null,"since":null,"kind":"",
"version":"","life":""},"workload-version":"","charm-version":"666",
"charm-profile":"","endpoint-bindings":null,"public-address":""}`, "\n", "", -1))
}
