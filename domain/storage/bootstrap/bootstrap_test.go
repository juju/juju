// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/state"
)

type bootstrapSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestCreateStoragePools(c *gc.C) {
	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs:    map[string]string{"foo": "barr"},
	}
	err := CreateStoragePools([]domainstorage.StoragePoolDetails{sp})(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	st := state.NewState(s.TxnRunnerFactory())
	result, err := st.ListStoragePools(context.Background(), domainstorage.StoragePoolFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []domainstorage.StoragePoolDetails{sp})
}
