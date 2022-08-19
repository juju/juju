// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type SecretsSuite struct {
	testing.StateSuite
	store     state.SecretsStore
	owner     *state.Application
	ownerUnit *state.Unit
	relation  *state.Relation
}

var _ = gc.Suite(&SecretsSuite{})

func (s *SecretsSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.store = state.NewSecretsStore(s.State)
	s.owner = s.Factory.MakeApplication(c, nil)
	s.ownerUnit = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.owner})
	app2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	ep1, err := s.owner.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	ep2, err := app2.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	s.relation = s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{ep1, ep2},
	})
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) TestCreate(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag().String(),
		Scope:   s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			ExpireTime:     ptr(now.Add(time.Hour)),
			Params:         nil,
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.URI.String(), gc.Equals, uri.String())
	md.URI = nil
	c.Assert(md, jc.DeepEquals, &secrets.SecretMetadata{
		Version:        1,
		Description:    "my secret",
		Label:          "foobar",
		RotatePolicy:   secrets.RotateDaily,
		NextRotateTime: ptr(now.Add(time.Minute)),
		ExpireTime:     ptr(now.Add(time.Hour)),
		OwnerTag:       s.owner.Tag().String(),
		ScopeTag:       s.ownerUnit.Tag().String(),
		ProviderID:     "",
		Revision:       1,
		CreateTime:     now,
		UpdateTime:     now,
	})

	_, err = s.store.CreateSecret(uri, p)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *SecretsSuite) TestCreateDyingOwner(c *gc.C) {
	err := s.owner.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err = s.store.CreateSecret(uri, cp)
	c.Assert(err, gc.ErrorMatches, `cannot create secret for owner "application-mysql" which is not alive`)
}

func (s *SecretsSuite) TestGetValueNotFound(c *gc.C) {
	uri, _ := secrets.ParseURI("secret:9m4e2mr0ui3e8a215n4g")
	_, err := s.store.GetSecretValue(uri, 666)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SecretsSuite) TestGetValue(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	p := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, p)
	c.Assert(err, jc.ErrorIsNil)

	val, err := s.store.GetSecretValue(md.URI, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *SecretsSuite) TestList(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	p := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			ExpireTime:     ptr(now.Add(time.Hour)),
			Params:         nil,
			Data:           map[string]string{"foo": "bar"},
		},
	}
	_, err := s.store.CreateSecret(uri, p)
	c.Assert(err, jc.ErrorIsNil)

	// Create another secret to ensure it is excluded.
	uri2 := secrets.NewURI()
	uri2.ControllerUUID = s.State.ControllerUUID()
	p.Owner = "application-wordpress"
	p.Scope = "application-wordpress"
	_, err = s.store.CreateSecret(uri2, p)
	c.Assert(err, jc.ErrorIsNil)

	list, err := s.store.ListSecrets(state.SecretsFilter{
		OwnerTag: s.owner.Tag().String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list, jc.DeepEquals, []*secrets.SecretMetadata{{
		URI:            uri,
		RotatePolicy:   secrets.RotateDaily,
		NextRotateTime: ptr(now.Add(time.Minute)),
		ExpireTime:     ptr(now.Add(time.Hour)),
		Version:        1,
		OwnerTag:       s.owner.Tag().String(),
		ScopeTag:       s.ownerUnit.Tag().String(),
		Description:    "my secret",
		Label:          "foobar",
		Provider:       "juju",
		ProviderID:     "",
		Revision:       1,
		CreateTime:     now,
		UpdateTime:     now,
	}})
}

func (s *SecretsSuite) TestUpdateNothing(c *gc.C) {
	up := state.UpdateSecretParams{}
	uri := secrets.NewURI()
	_, err := s.store.UpdateSecret(uri, up)
	c.Assert(err, gc.ErrorMatches, "must specify a new value or metadata to update a secret")
}

func (s *SecretsSuite) TestUpdateAll(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		Description:    ptr("big secret"),
		Label:          ptr("new label"),
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(2 * time.Minute)),
		Data:           newData,
	})
}

func (s *SecretsSuite) TestUpdateRotateInterval(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpdatedSecret(c, md, 1, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(time.Minute)),
	})
}

func (s *SecretsSuite) TestUpdateData(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		ProviderLabel: "juju",
		Version:       1,
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        newData,
	})
}

func (s *SecretsSuite) TestUpdateDataSetsLatestConsumerRevision(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		ProviderLabel: "juju",
		Version:       1,
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	cmd := &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 1,
	}
	err = s.State.SaveSecretConsumer(uri, "unit-mariadb-0", cmd)
	c.Assert(err, jc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        newData,
	})
	cmd, err = s.State.GetSecretConsumer(uri, "unit-mariadb-0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd, jc.DeepEquals, &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 1,
		LatestRevision:  2,
	})
}

func (s *SecretsSuite) TestUpdateDataSetsLatestConsumerRevisionConcurrentAdd(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		ProviderLabel: "juju",
		Version:       1,
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	cmd := &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 1,
	}
	err = s.State.SaveSecretConsumer(uri, "unit-mariadb-0", cmd)
	c.Assert(err, jc.ErrorIsNil)

	state.SetBeforeHooks(c, s.State, func() {
		err = s.State.SaveSecretConsumer(uri, "unit-mysql-0", cmd)
		c.Assert(err, jc.ErrorIsNil)
	})

	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        newData,
	})
	cmd, err = s.State.GetSecretConsumer(uri, "unit-mariadb-0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.LatestRevision, gc.Equals, 2)
	cmd, err = s.State.GetSecretConsumer(uri, "unit-mysql-0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.LatestRevision, gc.Equals, 2)
}

func (s *SecretsSuite) TestUpdateDataSetsLatestConsumerRevisionConcurrentRemove(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()
	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		ProviderLabel: "juju",
		Version:       1,
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	cmd := &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 1,
	}
	err = s.State.SaveSecretConsumer(uri, "unit-mariadb-0", cmd)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SaveSecretConsumer(uri, "unit-mysql-0", cmd)
	c.Assert(err, jc.ErrorIsNil)

	state.SetBeforeHooks(c, s.State, func() {
		consColl, closer := state.GetCollection(s.State, "secretConsumers")
		defer closer()
		err := consColl.Writeable().RemoveId(state.DocID(s.State, fmt.Sprintf("%s#unit-mysql-0", uri.ID)))
		c.Assert(err, jc.ErrorIsNil)

		err = state.IncSecretConsumerRefCount(s.State, uri, 1)
		c.Assert(err, jc.ErrorIsNil)
	})

	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        newData,
	})
	cmd, err = s.State.GetSecretConsumer(uri, "unit-mariadb-0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.LatestRevision, gc.Equals, 2)
	_, err = s.State.GetSecretConsumer(uri, "unit-mysql-0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SecretsSuite) assertUpdatedSecret(c *gc.C, original *secrets.SecretMetadata, expectedRevision int, update state.UpdateSecretParams) {
	expected := *original
	expected.Revision = expectedRevision
	if update.RotatePolicy != nil {
		expected.RotatePolicy = *update.RotatePolicy
		expected.NextRotateTime = update.NextRotateTime
	}
	if update.Description != nil {
		expected.Description = *update.Description
	}
	if update.Label != nil {
		expected.Label = *update.Label
	}

	s.Clock.Advance(time.Hour)
	updated := s.Clock.Now().Round(time.Second).UTC()
	expected.UpdateTime = updated
	md, err := s.store.UpdateSecret(original.URI, update)
	c.Assert(err, jc.ErrorIsNil)

	list, err := s.store.ListSecrets(state.SecretsFilter{})
	c.Assert(err, jc.ErrorIsNil)
	mc := jc.NewMultiChecker()
	mc.AddExpr(`(*_[_]).CreateTime`, jc.Almost, jc.ExpectedValue)
	mc.AddExpr(`(*_[_]).UpdateTime`, jc.Almost, jc.ExpectedValue)
	c.Assert(list, mc, []*secrets.SecretMetadata{&expected})
	expectedData := map[string]string{"foo": "bar"}
	if update.Data != nil {
		expectedData = update.Data
	}
	val, err := s.store.GetSecretValue(md.URI, expectedRevision)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val.EncodedValues(), jc.DeepEquals, expectedData)
}

func (s *SecretsSuite) TestUpdateConcurrent(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()

	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)

	state.SetBeforeHooks(c, s.State, func() {
		up := state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateYearly),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Params:         nil,
			Data:           map[string]string{"foo": "baz", "goodbye": "world"},
		}
		md, err = s.store.UpdateSecret(md.URI, up)
		c.Assert(err, jc.ErrorIsNil)
	})
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 3, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(time.Minute)),
		Data:           newData,
	})
}

func (s *SecretsSuite) TestGetSecretConsumer(c *gc.C) {
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	uri := secrets.NewURI()
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	uri.ControllerUUID = s.State.ControllerUUID()

	_, err = s.State.GetSecretConsumer(uri, "unit-mariadb-0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	md := &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 666,
	}
	err = s.State.SaveSecretConsumer(uri, "unit-mariadb-0", md)
	c.Assert(err, jc.ErrorIsNil)
	md2, err := s.State.GetSecretConsumer(uri, "unit-mariadb-0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md2, jc.DeepEquals, md)
	_, err = s.State.GetSecretConsumer(uri, "unit-mysql-0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SecretsSuite) TestSaveSecretConsumer(c *gc.C) {
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	uri := secrets.NewURI()
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	uri.ControllerUUID = s.State.ControllerUUID()
	md := &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 666,
	}
	err = s.State.SaveSecretConsumer(uri, "unit-mariadb-0", md)
	c.Assert(err, jc.ErrorIsNil)
	md2, err := s.State.GetSecretConsumer(uri, "unit-mariadb-0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md2, jc.DeepEquals, md)
	md.CurrentRevision = 668
	err = s.State.SaveSecretConsumer(uri, "unit-mariadb-0", md)
	c.Assert(err, jc.ErrorIsNil)
	md2, err = s.State.GetSecretConsumer(uri, "unit-mariadb-0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md2, jc.DeepEquals, md)
}

func (s *SecretsSuite) TestSaveSecretConsumerConcurrent(c *gc.C) {
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	uri := secrets.NewURI()
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	uri.ControllerUUID = s.State.ControllerUUID()
	md := &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 666,
	}
	state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SaveSecretConsumer(uri, "unit-mariadb-0", &secrets.SecretConsumerMetadata{CurrentRevision: 668})
		c.Assert(err, jc.ErrorIsNil)
	})
	err = s.State.SaveSecretConsumer(uri, "unit-mariadb-0", md)
	c.Assert(err, jc.ErrorIsNil)
	md2, err := s.State.GetSecretConsumer(uri, "unit-mariadb-0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md2, jc.DeepEquals, md)
}

func (s *SecretsSuite) TestSecretGrantAccess(c *gc.C) {
	uri := secrets.NewURI()
	subject := names.NewApplicationTag("wordpress")
	err := s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       s.relation.Tag(),
		Subject:     subject,
		Role:        secrets.RoleView,
	})
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err = s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       s.relation.Tag(),
		Subject:     subject,
		Role:        secrets.RoleView,
	})
	c.Assert(err, jc.ErrorIsNil)
	access, err := s.State.SecretAccess(uri, subject)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleView)
}

func (s *SecretsSuite) TestSecretGrantAccessDyingScope(c *gc.C) {
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure destroy only sets relation to dying.
	wordpress, err := s.State.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ru, err := s.relation.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.relation.DestroyWithForce(true, time.Second)
	c.Assert(err, jc.ErrorIsNil)

	subject := names.NewApplicationTag("wordpress")
	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       s.relation.Tag(),
		Subject:     subject,
		Role:        secrets.RoleView,
	})
	c.Assert(err, gc.ErrorMatches, `cannot grant access to secret in scope of "relation-wordpress.db#mysql.server" which is not alive`)
}

func (s *SecretsSuite) TestSecretRevokeAccess(c *gc.C) {
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	subject := names.NewApplicationTag("wordpress")
	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       s.relation.Tag(),
		Subject:     subject,
		Role:        secrets.RoleView,
	})
	c.Assert(err, jc.ErrorIsNil)
	access, err := s.State.SecretAccess(uri, subject)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleView)

	err = s.State.RevokeSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Subject:     subject,
	})
	c.Assert(err, jc.ErrorIsNil)
	access, err = s.State.SecretAccess(uri, subject)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleNone)

	err = s.State.RevokeSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Subject:     subject,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestDelete(c *gc.C) {
	subject := names.NewApplicationTag("wordpress")
	create := func() *secrets.URI {
		uri := secrets.NewURI()
		uri.ControllerUUID = s.State.ControllerUUID()
		now := s.Clock.Now().Round(time.Second).UTC()
		cp := state.CreateSecretParams{
			Version: 1,
			Owner:   s.owner.Tag().String(),
			Scope:   s.ownerUnit.Tag().String(),
			UpdateSecretParams: state.UpdateSecretParams{
				LeaderToken:    &fakeToken{},
				RotatePolicy:   ptr(secrets.RotateDaily),
				NextRotateTime: ptr(now.Add(time.Hour)),
				Data:           map[string]string{"foo": "bar"},
			},
		}
		_, err := s.store.CreateSecret(uri, cp)
		c.Assert(err, jc.ErrorIsNil)
		cmd := &secrets.SecretConsumerMetadata{
			Label:           "foobar",
			CurrentRevision: 1,
		}
		err = s.State.SaveSecretConsumer(uri, "unit-mariadb-0", cmd)
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
			LeaderToken: &fakeToken{},
			Scope:       s.relation.Tag(),
			Subject:     subject,
			Role:        secrets.RoleView,
		})
		c.Assert(err, jc.ErrorIsNil)
		return uri
	}
	uri1 := create()
	uri2 := create()

	err := s.store.DeleteSecret(uri1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.store.GetSecretValue(uri1, 1)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = s.store.DeleteSecret(uri1)
	c.Assert(err, jc.ErrorIsNil)

	// Check that other secret info remains intact.
	secretRevisionsCollection, closer := state.GetRawCollection(s.State, "secretRevisions")
	defer closer()
	n, err := secretRevisionsCollection.FindId(uri2.ID + "/1").Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
	n, err = secretRevisionsCollection.Find(nil).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)

	secretRotateCollection, closer := state.GetRawCollection(s.State, "secretRotate")
	defer closer()
	n, err = secretRotateCollection.FindId(uri2.ID).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
	n, err = secretRotateCollection.Find(nil).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)

	secretConsumersCollection, closer := state.GetRawCollection(s.State, "secretConsumers")
	defer closer()
	n, err = secretConsumersCollection.FindId(state.DocID(s.State, uri2.ID) + "#unit-mariadb-0").Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
	n, err = secretConsumersCollection.Find(nil).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)

	secretPermissionsCollection, closer := state.GetRawCollection(s.State, "secretPermissions")
	defer closer()
	n, err = secretPermissionsCollection.FindId(state.DocID(s.State, uri2.ID) + "#application-wordpress").Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
	n, err = secretPermissionsCollection.Find(nil).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)

	refCountsCollection, closer := state.GetRawCollection(s.State, "refcounts")
	defer closer()
	n, err = refCountsCollection.FindId(state.DocID(s.State, uri2.ID) + "#consumer").Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
	n, err = refCountsCollection.FindId(state.DocID(s.State, uri1.ID) + "#consumer").Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 0)
}

func (s *SecretsSuite) TestSecretRotated(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()

	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SecretRotated(uri, now)
	c.Assert(err, jc.ErrorIsNil)

	rotated := state.GetSecretRotateTime(c, s.State, md.URI.ID)
	c.Assert(rotated, gc.Equals, now.Round(time.Second).UTC())
}

func (s *SecretsSuite) TestSecretRotatedConcurrent(c *gc.C) {
	uri := secrets.NewURI()
	uri.ControllerUUID = s.State.ControllerUUID()

	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version:       1,
		ProviderLabel: "juju",
		Owner:         s.owner.Tag().String(),
		Scope:         s.ownerUnit.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	later := now.Add(time.Hour)
	state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SecretRotated(uri, later)
		c.Assert(err, jc.ErrorIsNil)
	})

	err = s.State.SecretRotated(uri, now)
	c.Assert(err, jc.ErrorIsNil)

	rotated := state.GetSecretRotateTime(c, s.State, md.URI.ID)
	c.Assert(rotated, gc.Equals, later.Round(time.Second))
}

type SecretsRotationWatcherSuite struct {
	testing.StateSuite
	store state.SecretsStore

	owner *state.Application
}

var _ = gc.Suite(&SecretsRotationWatcherSuite{})

func (s *SecretsRotationWatcherSuite) SetUpTest(c *gc.C) {
	c.Skip("rotation not implemented")
	s.StateSuite.SetUpTest(c)
	s.store = state.NewSecretsStore(s.State)
	s.owner = s.Factory.MakeApplication(c, nil)
}

func (s *SecretsRotationWatcherSuite) setupWatcher(c *gc.C) (state.SecretsRotationWatcher, *secrets.URI) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag().String(),
		Scope:   s.owner.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Hour)),
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	w := s.State.WatchSecretsRotationChanges("application-mariadb")

	wc := testing.NewSecretsRotationWatcherC(c, w)
	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: time.Hour,
		LastRotateTime: now,
	})
	wc.AssertNoChange()
	return w, uri
}

func (s *SecretsRotationWatcherSuite) TestWatchInitialEvent(c *gc.C) {
	w, _ := s.setupWatcher(c)
	testing.AssertStop(c, w)
}

func (s *SecretsRotationWatcherSuite) TestWatchSingleUpdate(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(time.Minute)),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: time.Hour,
		LastRotateTime: md.CreateTime.UTC(),
	})
	wc.AssertNoChange()
}

func (s *SecretsRotationWatcherSuite) TestWatchDelete(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, w)
	defer testing.AssertStop(c, w)

	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:  &fakeToken{},
		RotatePolicy: ptr(secrets.RotateNever),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: 0,
	})
	wc.AssertNoChange()
}

func (s *SecretsRotationWatcherSuite) TestWatchMultipleUpdatesSameSecret(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		RotatePolicy:   ptr(secrets.RotateYearly),
		NextRotateTime: ptr(now.Add(time.Minute)),
	})
	c.Assert(err, jc.ErrorIsNil)
	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(time.Minute)),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: time.Hour,
		LastRotateTime: md.CreateTime.UTC(),
	})
	wc.AssertNoChange()
}

func (s *SecretsRotationWatcherSuite) TestWatchMultipleUpdatesSameSecretDeleted(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(time.Minute)),
	})
	c.Assert(err, jc.ErrorIsNil)
	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:  &fakeToken{},
		RotatePolicy: ptr(secrets.RotateNever),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: 0,
	})
	wc.AssertNoChange()
}

func (s *SecretsRotationWatcherSuite) TestWatchMultipleUpdates(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsRotationWatcherC(c, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(now.Add(time.Minute)),
	})
	c.Assert(err, jc.ErrorIsNil)

	uri2 := secrets.NewURI()
	md2, err := s.store.CreateSecret(uri2, state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag().String(),
		Scope:   s.owner.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateHourly),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:  &fakeToken{},
		RotatePolicy: ptr(secrets.RotateNever),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(watcher.SecretRotationChange{
		URI:            md2.URI.Raw(),
		RotateInterval: time.Hour,
		LastRotateTime: md2.CreateTime.UTC(),
	}, watcher.SecretRotationChange{
		URI:            md.URI.Raw(),
		RotateInterval: 0,
	})
	wc.AssertNoChange()
}

type SecretsWatcherSuite struct {
	testing.StateSuite
	store state.SecretsStore

	owner *state.Application
}

var _ = gc.Suite(&SecretsWatcherSuite{})

func (s *SecretsWatcherSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.store = state.NewSecretsStore(s.State)
	s.owner = s.Factory.MakeApplication(c, nil)
}

func (s *SecretsWatcherSuite) setupWatcher(c *gc.C) (state.StringsWatcher, *secrets.URI) {
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag().String(),
		Scope:   s.owner.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
	w := s.State.WatchConsumedSecretsChanges("unit-mariadb-0")

	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()

	err = s.State.SaveSecretConsumer(uri, "unit-mariadb-0", &secrets.SecretConsumerMetadata{CurrentRevision: 1})
	c.Assert(err, jc.ErrorIsNil)
	// No event until rev > 1.
	wc.AssertNoChange()
	return w, uri
}

func (s *SecretsWatcherSuite) TestWatcherStartStop(c *gc.C) {
	w, _ := s.setupWatcher(c)
	testing.AssertStop(c, w)
}

func (s *SecretsWatcherSuite) TestWatchSingleUpdate(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewStringsWatcherC(c, w)
	defer testing.AssertStop(c, w)

	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        secrets.SecretData{"foo": "bar2"},
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(uri.String())
	wc.AssertNoChange()
}

func (s *SecretsWatcherSuite) TestWatchMultipleSecrets(c *gc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewStringsWatcherC(c, w)
	defer testing.AssertStop(c, w)

	uri2 := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag().String(),
		Scope:   s.owner.Tag().String(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo2": "bar"},
		},
	}
	_, err := s.store.CreateSecret(uri2, cp)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SaveSecretConsumer(uri2, "unit-mariadb-0", &secrets.SecretConsumerMetadata{CurrentRevision: 1})
	c.Assert(err, jc.ErrorIsNil)
	// No event until rev > 1.
	wc.AssertNoChange()

	_, err = s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        secrets.SecretData{"foo": "bar2"},
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(uri.String())
	wc.AssertNoChange()

	_, err = s.store.UpdateSecret(uri2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        secrets.SecretData{"foo2": "bar2"},
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(uri2.String())
	wc.AssertNoChange()
}
