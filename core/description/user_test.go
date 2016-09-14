// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/testing"
)

type UserSerializationSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&UserSerializationSuite{})

func (*UserSerializationSuite) TestNil(c *gc.C) {
	_, err := importUsers(nil)
	c.Check(err, gc.ErrorMatches, "users version schema check failed: .*")
}

func (*UserSerializationSuite) TestMissingVersion(c *gc.C) {
	_, err := importUsers(map[string]interface{}{
		"users": []interface{}{},
	})
	c.Check(err.Error(), gc.Equals, "users version schema check failed: version: expected int, got nothing")
}

func (*UserSerializationSuite) TestMissingUsers(c *gc.C) {
	_, err := importUsers(map[string]interface{}{
		"version": 1,
	})
	c.Check(err.Error(), gc.Equals, "users version schema check failed: users: expected list, got nothing")
}

func (*UserSerializationSuite) TestNonIntVersion(c *gc.C) {
	_, err := importUsers(map[string]interface{}{
		"version": "hello",
		"users":   []interface{}{},
	})
	c.Check(err.Error(), gc.Equals, `users version schema check failed: version: expected int, got string("hello")`)
}

func (*UserSerializationSuite) TestUnknownVersion(c *gc.C) {
	_, err := importUsers(map[string]interface{}{
		"version": 42,
		"users":   []interface{}{},
	})
	c.Check(err.Error(), gc.Equals, `version 42 not valid`)
}

func (*UserSerializationSuite) TestParsingSerializedData(c *gc.C) {
	lastConn := time.Date(2016, 1, 15, 12, 0, 0, 0, time.UTC)
	initial := users{
		Version: 1,
		Users_: []*user{
			&user{
				Name_:           "admin@local",
				CreatedBy_:      "admin@local",
				DateCreated_:    time.Date(2015, 10, 9, 12, 34, 56, 0, time.UTC),
				LastConnection_: &lastConn,
			},
			&user{
				Name_:        "read-only@local",
				DisplayName_: "A read only user",
				CreatedBy_:   "admin@local",
				DateCreated_: time.Date(2015, 10, 9, 12, 34, 56, 0, time.UTC),
				Access_:      "read",
			},
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("%s", bytes)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	users, err := importUsers(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(users, jc.DeepEquals, initial.Users_)
}
