// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stdtesting "testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/tc"

	macaroonerrors "github.com/juju/juju/domain/macaroon/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

var (
	testKey1 = bakery.MustGenerateKey()
	testKey2 = bakery.MustGenerateKey()
	testKey3 = bakery.MustGenerateKey()
	testKey4 = bakery.MustGenerateKey()
)

type configStateSuite struct {
	schematesting.ControllerSuite
}

func TestConfigStateSuite(t *stdtesting.T) { tc.Run(t, &configStateSuite{}) }
func (s *configStateSuite) TestInitialise(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.InitialiseBakeryConfig(c.Context(), testKey1, testKey2, testKey3, testKey4)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *configStateSuite) TestInitialiseMultipleTimesFails(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.InitialiseBakeryConfig(c.Context(), testKey1, testKey2, testKey3, testKey4)
	c.Assert(err, tc.ErrorIsNil)

	err = st.InitialiseBakeryConfig(c.Context(), testKey1, testKey2, testKey3, testKey4)
	c.Assert(err, tc.ErrorIs, macaroonerrors.BakeryConfigAlreadyInitialised)
}

func (s *configStateSuite) TestGetKeys(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.InitialiseBakeryConfig(c.Context(), testKey1, testKey2, testKey3, testKey4)
	c.Assert(err, tc.ErrorIsNil)

	keypair, err := st.GetLocalUsersKey(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(keypair, tc.DeepEquals, testKey1)

	keypair, err = st.GetLocalUsersThirdPartyKey(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(keypair, tc.DeepEquals, testKey2)

	keypair, err = st.GetExternalUsersThirdPartyKey(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(keypair, tc.DeepEquals, testKey3)

	keypair, err = st.GetOffersThirdPartyKey(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(keypair, tc.DeepEquals, testKey4)
}

func (s *configStateSuite) TestGetKeysUninitialised(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetLocalUsersKey(c.Context())
	c.Check(err, tc.ErrorIs, macaroonerrors.NotInitialised)

	_, err = st.GetLocalUsersThirdPartyKey(c.Context())
	c.Check(err, tc.ErrorIs, macaroonerrors.NotInitialised)

	_, err = st.GetExternalUsersThirdPartyKey(c.Context())
	c.Check(err, tc.ErrorIs, macaroonerrors.NotInitialised)

	_, err = st.GetOffersThirdPartyKey(c.Context())
	c.Check(err, tc.ErrorIs, macaroonerrors.NotInitialised)
}
