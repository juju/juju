// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"encoding/json"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/mgo/v3"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/mongo"
	jujutesting "github.com/juju/juju/testing"
)

type ConfigSuite struct {
	jujutesting.BaseSuite
	testing.Stub

	collectionGetter func(name string) (mongo.Collection, func())
	collection       mockCollection
	closeCollection  func()

	bakeryDocResult bakeryConfigDoc
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.Stub.ResetCalls()
	s.collection = mockCollection{
		Stub: &s.Stub,
		one: func(q *mockQuery, result *interface{}) error {
			id := q.id.(string)
			if id != "bakeryConfig" {
				return mgo.ErrNotFound
			}
			*(*result).(*bakeryConfigDoc) = s.bakeryDocResult
			return nil
		},
	}
	s.closeCollection = func() {
		s.AddCall("Close")
		s.PopNoErr()
	}
	s.collectionGetter = func(collection string) (mongo.Collection, func()) {
		s.AddCall("GetCollection", collection)
		s.PopNoErr()
		return &s.collection, s.closeCollection
	}
}

func (s *ConfigSuite) TestInitialiseBakeryConfigOp(c *gc.C) {
	bakeryConfig := NewBakeryConfig("test", s.collectionGetter)
	op, err := bakeryConfig.InitialiseBakeryConfigOp()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.C, gc.Equals, "test")

	doc, ok := op.Insert.(*bakeryConfigDoc)
	c.Assert(ok, jc.IsTrue)
	var key bakery.KeyPair
	err = json.Unmarshal([]byte(doc.LocalUsersKey), &key)
	c.Assert(err, jc.ErrorIsNil)
	err = json.Unmarshal([]byte(doc.OffersThirdPartyKey), &key)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ConfigSuite) TestLocalUsersKey(c *gc.C) {
	s.bakeryDocResult = bakeryConfigDoc{
		LocalUsersKey:              `{"public":"XXy70HKjZ6SbrW0h6zb5xkQYzUAvarTDFrl4//7wgUo=","private":"AwHI3v9AQjbAzhZx0JBjqaPYhVJ5Ksi+PWog4rNwS9Y="}`,
		LocalUsersThirdPartyKey:    "x",
		ExternalUsersThirdPartyKey: "x",
		OffersThirdPartyKey:        "x",
	}
	bakeryConfig := NewBakeryConfig("test", s.collectionGetter)
	key, err := bakeryConfig.GetLocalUsersKey()
	c.Assert(err, jc.ErrorIsNil)
	keyBytes, err := json.Marshal(key)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCalls(c, []testing.StubCall{
		{"GetCollection", []interface{}{"test"}},
		{"FindId", []interface{}{"bakeryConfig"}},
		{"One", []interface{}{&bakeryConfigDoc{
			LocalUsersKey:              string(keyBytes),
			LocalUsersThirdPartyKey:    "x",
			ExternalUsersThirdPartyKey: "x",
			OffersThirdPartyKey:        "x",
		}}},
		{"Close", nil},
	})
}

func (s *ConfigSuite) TestLocalUsersThirdPartyKey(c *gc.C) {
	s.bakeryDocResult = bakeryConfigDoc{
		LocalUsersKey:              "x",
		LocalUsersThirdPartyKey:    `{"public":"XXy70HKjZ6SbrW0h6zb5xkQYzUAvarTDFrl4//7wgUo=","private":"AwHI3v9AQjbAzhZx0JBjqaPYhVJ5Ksi+PWog4rNwS9Y="}`,
		ExternalUsersThirdPartyKey: "x",
		OffersThirdPartyKey:        "x",
	}
	bakeryConfig := NewBakeryConfig("test", s.collectionGetter)
	key, err := bakeryConfig.GetLocalUsersThirdPartyKey()
	c.Assert(err, jc.ErrorIsNil)
	keyBytes, err := json.Marshal(key)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCalls(c, []testing.StubCall{
		{"GetCollection", []interface{}{"test"}},
		{"FindId", []interface{}{"bakeryConfig"}},
		{"One", []interface{}{&bakeryConfigDoc{
			LocalUsersKey:              "x",
			LocalUsersThirdPartyKey:    string(keyBytes),
			ExternalUsersThirdPartyKey: "x",
			OffersThirdPartyKey:        "x",
		}}},
		{"Close", nil},
	})
}

func (s *ConfigSuite) TestExternalUsersThirdPartyKey(c *gc.C) {
	s.bakeryDocResult = bakeryConfigDoc{
		LocalUsersKey:              "x",
		LocalUsersThirdPartyKey:    "x",
		ExternalUsersThirdPartyKey: `{"public":"XXy70HKjZ6SbrW0h6zb5xkQYzUAvarTDFrl4//7wgUo=","private":"AwHI3v9AQjbAzhZx0JBjqaPYhVJ5Ksi+PWog4rNwS9Y="}`,
		OffersThirdPartyKey:        "x",
	}
	bakeryConfig := NewBakeryConfig("test", s.collectionGetter)
	key, err := bakeryConfig.GetExternalUsersThirdPartyKey()
	c.Assert(err, jc.ErrorIsNil)
	keyBytes, err := json.Marshal(key)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCalls(c, []testing.StubCall{
		{"GetCollection", []interface{}{"test"}},
		{"FindId", []interface{}{"bakeryConfig"}},
		{"One", []interface{}{&bakeryConfigDoc{
			LocalUsersKey:              "x",
			LocalUsersThirdPartyKey:    "x",
			ExternalUsersThirdPartyKey: string(keyBytes),
			OffersThirdPartyKey:        "x",
		}}},
		{"Close", nil},
	})
}

func (s *ConfigSuite) TestOffersThirdPartyKey(c *gc.C) {
	s.bakeryDocResult = bakeryConfigDoc{
		LocalUsersKey:              "x",
		LocalUsersThirdPartyKey:    "x",
		ExternalUsersThirdPartyKey: "x",
		OffersThirdPartyKey:        `{"public":"XXy70HKjZ6SbrW0h6zb5xkQYzUAvarTDFrl4//7wgUo=","private":"AwHI3v9AQjbAzhZx0JBjqaPYhVJ5Ksi+PWog4rNwS9Y="}`,
	}
	bakeryConfig := NewBakeryConfig("test", s.collectionGetter)
	key, err := bakeryConfig.GetOffersThirdPartyKey()
	c.Assert(err, jc.ErrorIsNil)
	keyBytes, err := json.Marshal(key)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCalls(c, []testing.StubCall{
		{"GetCollection", []interface{}{"test"}},
		{"FindId", []interface{}{"bakeryConfig"}},
		{"One", []interface{}{&bakeryConfigDoc{
			LocalUsersKey:              "x",
			LocalUsersThirdPartyKey:    "x",
			ExternalUsersThirdPartyKey: "x",
			OffersThirdPartyKey:        string(keyBytes),
		}}},
		{"Close", nil},
	})
}
