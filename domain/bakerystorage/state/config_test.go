// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/changestream/testing"
)

var (
	testKey1 = bakery.MustGenerateKey()
	testKey2 = bakery.MustGenerateKey()
	testKey3 = bakery.MustGenerateKey()
	testKey4 = bakery.MustGenerateKey()
)

type stateSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestInitialise(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.InitialiseBakeryConfig(context.Background(), testKey1, testKey2, testKey3, testKey4)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestInitialiseMultipleTimesFails(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.InitialiseBakeryConfig(context.Background(), testKey1, testKey2, testKey3, testKey4)
	c.Assert(err, jc.ErrorIsNil)

	err = st.InitialiseBakeryConfig(context.Background(), testKey1, testKey2, testKey3, testKey4)
	c.Assert(err, jc.ErrorIs, BakeryConfigAlreadyInitialised)
}

func (s *stateSuite) TestGetKeys(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.InitialiseBakeryConfig(context.Background(), testKey1, testKey2, testKey3, testKey4)
	c.Assert(err, jc.ErrorIsNil)

	keypair, err := st.GetLocalUsersKey(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(keypair, gc.DeepEquals, testKey1)

	keypair, err = st.GetLocalUsersThirdPartyKey(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(keypair, gc.DeepEquals, testKey2)

	keypair, err = st.GetExternalUsersThirdPartyKey(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(keypair, gc.DeepEquals, testKey3)

	keypair, err = st.GetOffersThirdPartyKey(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(keypair, gc.DeepEquals, testKey4)
}

func (s *stateSuite) TestGetKeysUninitialised(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetLocalUsersKey(context.Background())
	c.Check(err, jc.ErrorIs, errors.NotYetAvailable)

	_, err = st.GetLocalUsersThirdPartyKey(context.Background())
	c.Check(err, jc.ErrorIs, errors.NotYetAvailable)

	_, err = st.GetExternalUsersThirdPartyKey(context.Background())
	c.Check(err, jc.ErrorIs, errors.NotYetAvailable)

	_, err = st.GetOffersThirdPartyKey(context.Background())
	c.Check(err, jc.ErrorIs, errors.NotYetAvailable)
}
