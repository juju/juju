// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common/crossmodel"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	coretesting "github.com/juju/juju/testing"
)

type mockBakery struct {
	*bakery.Bakery
}

func (m *mockBakery) ExpireStorageAfter(time.Duration) (authentication.ExpirableStorageBakery, error) {
	return m, nil
}

func (m *mockBakery) Auth(_ context.Context, mss ...macaroon.Slice) *bakery.AuthChecker {
	return m.Bakery.Checker.Auth(mss...)
}

func (m *mockBakery) NewMacaroon(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error) {
	return m.Bakery.Oven.NewMacaroon(ctx, version, caveats, ops...)
}

type mockState struct {
	crossmodel.Backend
	tag         names.ModelTag
	permissions map[string]permission.Access
}

func (m *mockState) ApplicationOfferForUUID(offerUUID string) (*jujucrossmodel.ApplicationOffer, error) {
	return &jujucrossmodel.ApplicationOffer{OfferUUID: offerUUID}, nil
}

func (m *mockState) UserPermission(subject names.UserTag, target names.Tag) (permission.Access, error) {
	perm, ok := m.permissions[target.Id()+":"+subject.Id()]
	if !ok {
		return permission.NoAccess, nil
	}
	return perm, nil
}

func (m *mockState) GetOfferAccess(offerUUID string, user names.UserTag) (permission.Access, error) {
	perm, ok := m.permissions[offerUUID+":"+user.Id()]
	if !ok {
		return permission.NoAccess, nil
	}
	return perm, nil
}

func (m *mockState) ControllerTag() names.ControllerTag {
	return coretesting.ControllerTag
}

func (m *mockState) ModelTag() names.ModelTag {
	return m.tag
}
