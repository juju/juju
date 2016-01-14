// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/schema"
)

type users struct {
	Version int     `yaml:"version"`
	Users_  []*user `yaml:"users"`
}

type UserArgs struct {
	Name           names.UserTag
	DisplayName    string
	CreatedBy      names.UserTag
	DateCreated    time.Time
	LastConnection time.Time
	ReadOnly       bool
}

func NewUser(args UserArgs) User {
	u := &user{
		Name_:           args.Name.Canonical(),
		DisplayName_:    args.DisplayName,
		CreatedBy_:      args.CreatedBy.Canonical(),
		DateCreated_:    args.DateCreated.Format(time.RFC3339Nano),
		ReadOnly_:       args.ReadOnly,
	}
	empty:=time.Time{}
	if args.LastConnection != empty {
		u.LastConnection_ = args.LastConnection.Format(time.RFC3339Nano)
	}
	return u
}

type user struct {
	Name_           string    `yaml:"name"`
	DisplayName_    string    `yaml:"display-name"`
	CreatedBy_      string    `yaml:"created-by"`
	DateCreated_    string `yaml:"date-created"`
	LastConnection_ string `yaml:"last-connection,omitempty"`
	ReadOnly_       bool      `yaml:"read-only,omitempty"`
}

func (u *user) Name() names.UserTag {
	return names.NewUserTag(u.Name_)
}

func (u *user) DisplayName() string {
	return u.DisplayName_
}

func (u *user) CreatedBy() names.UserTag {
	return names.NewUserTag(u.CreatedBy_)
}

func (u *user) DateCreated() time.Time {
	return u.DateCreated_
}

func (u *user) LastConnection() time.Time {
	if u.LastConnection_ != "" {

	}
	return time.Time{}
}

func (u *user) ReadOnly() bool {
	return u.ReadOnly_
}

func importUsers(source map[string]interface{}) ([]*user, error) {
	checker := versionedChecker("users")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "users version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := userDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["users"].([]interface{})
	return importUserList(sourceList, importFunc)
}

func importUserList(sourceList []interface{}, importFunc userDeserializationFunc) ([]*user, error) {
	result := make([]*user, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for user %d, %T", i, value)
		}
		user, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "user %d", i)
		}
		result = append(result, user)
	}
	return result, nil
}

type userDeserializationFunc func(map[string]interface{}) (*user, error)

var userDeserializationFuncs = map[int]userDeserializationFunc{
	1: importUserV1,
}

func importUserV1(source map[string]interface{}) (*user, error) {
	result := &user{}

	fields := schema.Fields{
		"name":         schema.String(),
		"display-name":         schema.String(),
		"created-by":         schema.String(),
		"read-only": schema.Bool(),
	}

	DateCreated_    time.Time `yaml:"date-created"`
	LastConnection_ time.Time `yaml:"last-connection"`

	checker := schema.FieldMap(fields, nil) // no defaults

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "user v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	result.Id_ = valid["id"].(string)
	userList := valid["containers"].([]interface{})
	users, err := importUserList(userList, importUserV1)
	if err != nil {
		return nil, errors.Annotatef(err, "containers")
	}
	result.Containers_ = users

	return result, nil

}
