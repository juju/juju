// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type importSuite struct {
	testhelpers.IsolationSuite

	state     *MockState
	modelUUID model.UUID
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.modelUUID = tc.Must0(c, model.NewUUID)
	return ctrl
}

func (s *importSuite) TestImportAuthorizedKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	alice := usertesting.GenNewName(c, "alice")
	bob := usertesting.GenNewName(c, "bob")
	aliceUUID := usertesting.GenUserUUID(c)
	bobUUID := usertesting.GenUserUUID(c)
	resolve := func(_ context.Context, name user.Name) (user.UUID, error) {
		switch name {
		case alice:
			return aliceUUID, nil
		case bob:
			return bobUUID, nil
		}
		return "", errors.Errorf("unexpected user %q", name)
	}

	s.state.EXPECT().AddPublicKeysForUser(gomock.Any(), s.modelUUID, aliceUUID, gomock.Any()).Return(nil)
	s.state.EXPECT().AddPublicKeysForUser(gomock.Any(), s.modelUUID, bobUUID, gomock.Any()).Return(nil)

	err := NewService(s.modelUUID, s.state).ImportAuthorizedKeys(
		c.Context(),
		[]coremodelmigration.ModelAuthorizedKey{{
			Username:  alice.Name(),
			PublicKey: testingPublicKeys[0],
		}, {
			Username:  bob.Name(),
			PublicKey: testingPublicKeys[1],
		}, {
			Username:  alice.Name(),
			PublicKey: testingPublicKeys[2],
		}, {
			Username:  "inactive",
			PublicKey: testingPublicKeys[0],
		}},
		set.NewStrings("inactive"),
		resolve,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportAuthorizedKeysEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resolve := func(_ context.Context, _ user.Name) (user.UUID, error) {
		return "", errors.New("resolve should not be called")
	}
	err := NewService(s.modelUUID, s.state).ImportAuthorizedKeys(
		c.Context(), nil, set.NewStrings(), resolve,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportAuthorizedKeysUserError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := errors.New("boom")
	resolve := func(_ context.Context, _ user.Name) (user.UUID, error) {
		return "", expected
	}
	err := NewService(s.modelUUID, s.state).ImportAuthorizedKeys(
		c.Context(),
		[]coremodelmigration.ModelAuthorizedKey{{
			Username:  "alice",
			PublicKey: testingPublicKeys[0],
		}},
		set.NewStrings(),
		resolve,
	)
	c.Assert(err, tc.ErrorIs, expected)
}
