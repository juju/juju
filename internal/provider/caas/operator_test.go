// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/provider/caas"
	"github.com/juju/juju/testing"
)

type OperatorSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&OperatorSuite{})

func (s *OperatorSuite) TestOperatorInfo(c *gc.C) {
	info := caas.OperatorInfo{
		CACert:     "ca cert",
		Cert:       "cert",
		PrivateKey: "private key",
	}
	marshaled, err := info.Marshal()
	c.Assert(err, jc.ErrorIsNil)
	unmarshaledInfo, err := caas.UnmarshalOperatorInfo(marshaled)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*unmarshaledInfo, jc.DeepEquals, info)
}

func (s *OperatorSuite) TestOperatorClientInfo(c *gc.C) {
	info := caas.OperatorClientInfo{
		ServiceAddress: "1.2.3.4",
		Token:          "token",
	}
	marshaled, err := info.Marshal()
	c.Assert(err, jc.ErrorIsNil)
	unmarshaledInfo, err := caas.UnmarshalOperatorClientInfo(marshaled)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*unmarshaledInfo, jc.DeepEquals, info)
}
