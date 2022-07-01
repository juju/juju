// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/caas/kubernetes/provider"
)

var _ = gc.Suite(&secretsSuite{})

type secretsSuite struct {
	BaseSuite
}

func (s *secretsSuite) TestProcessSecretData(c *gc.C) {
	o, err := provider.ProcessSecretData(
		map[string]string{
			"username": "YWRtaW4=",
			"password": "MWYyZDFlMmU2N2Rm",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, gc.DeepEquals, map[string][]byte{
		"username": []byte("admin"),
		"password": []byte("1f2d1e2e67df"),
	})
}
