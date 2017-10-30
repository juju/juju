// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common/crossmodel"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
	coretesting "github.com/juju/juju/testing"
)

type mockBakeryService struct {
	mu sync.Mutex
	*bakery.Service
	clock   *testing.Clock
	expired chan struct{}
}

func (m *mockBakeryService) ExpireStorageAt(at time.Time) (authentication.ExpirableStorageBakeryService, error) {
	now := m.clock.Now()
	delay := at.UnixNano() - now.UnixNano()
	go func() {
		select {
		case <-m.clock.After(time.Duration(delay) / time.Nanosecond):
			m.mu.Lock()
			m.Service = m.WithRootKeyStore(bakery.NewMemRootKeyStorage())
			m.mu.Unlock()
			m.expired <- struct{}{}
		}
	}()
	return m, nil
}

func (m *mockBakeryService) NewMacaroon(id string, rootKey []byte, caveats []checkers.Caveat) (*macaroon.Macaroon, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Service.NewMacaroon(id, rootKey, caveats)
}

func (m *mockBakeryService) CheckAny(mss []macaroon.Slice, assert map[string]string, checker checkers.Checker) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Service.CheckAny(mss, assert, checker)
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
