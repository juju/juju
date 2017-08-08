// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/permission"
	coretesting "github.com/juju/juju/testing"
)

type mockBakeryService struct {
	*bakery.Service
}

func (m *mockBakeryService) ExpireStorageAt(time.Time) (authentication.ExpirableStorageBakeryService, error) {
	return m, nil
}

type mockStatePool struct {
	st map[string]crossmodel.Backend
}

func (st *mockStatePool) Get(modelUUID string) (crossmodel.Backend, func(), error) {
	backend, ok := st.st[modelUUID]
	if !ok {
		return nil, nil, errors.NotFoundf("model for uuid %s", modelUUID)
	}
	return backend, func() {}, nil
}

type mockState struct {
	crossmodel.Backend
	tag         names.ModelTag
	permissions map[string]permission.Access
}

func (m *mockState) UserPermission(subject names.UserTag, target names.Tag) (permission.Access, error) {
	perm, ok := m.permissions[target.Id()+":"+subject.Id()]
	if !ok {
		return permission.NoAccess, nil
	}
	return perm, nil
}

func (m *mockState) GetOfferAccess(offer names.ApplicationOfferTag, user names.UserTag) (permission.Access, error) {
	perm, ok := m.permissions[offer.Id()+":"+user.Id()]
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
