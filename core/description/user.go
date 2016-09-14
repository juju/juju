// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/names.v2"
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
	Access         string
}

func newUser(args UserArgs) *user {
	u := &user{
		Name_:        args.Name.Canonical(),
		DisplayName_: args.DisplayName,
		CreatedBy_:   args.CreatedBy.Canonical(),
		DateCreated_: args.DateCreated,
		Access_:      args.Access,
	}
	if !args.LastConnection.IsZero() {
		value := args.LastConnection
		u.LastConnection_ = &value
	}
	return u
}

type user struct {
	Name_        string    `yaml:"name"`
	DisplayName_ string    `yaml:"display-name,omitempty"`
	CreatedBy_   string    `yaml:"created-by"`
	DateCreated_ time.Time `yaml:"date-created"`
	Access_      string    `yaml:"access"`
	// Can't use omitempty with time.Time, it just doesn't work,
	// so use a pointer in the struct.
	LastConnection_ *time.Time `yaml:"last-connection,omitempty"`
}

// Name implements User.
func (u *user) Name() names.UserTag {
	return names.NewUserTag(u.Name_)
}

// DisplayName implements User.
func (u *user) DisplayName() string {
	return u.DisplayName_
}

// CreatedBy implements User.
func (u *user) CreatedBy() names.UserTag {
	return names.NewUserTag(u.CreatedBy_)
}

// DateCreated implements User.
func (u *user) DateCreated() time.Time {
	return u.DateCreated_
}

// LastConnection implements User.
func (u *user) LastConnection() time.Time {
	var zero time.Time
	if u.LastConnection_ == nil {
		return zero
	}
	return *u.LastConnection_
}

// Access implements User.
func (u *user) Access() string {
	return u.Access_
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
	fields := schema.Fields{
		"name":            schema.String(),
		"display-name":    schema.String(),
		"created-by":      schema.String(),
		"read-only":       schema.Bool(),
		"date-created":    schema.Time(),
		"last-connection": schema.Time(),
		"access":          schema.String(),
	}

	// Some values don't have to be there.
	defaults := schema.Defaults{
		"display-name":    "",
		"last-connection": time.Time{},
		"read-only":       false,
	}
	checker := schema.FieldMap(fields, defaults)
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "user v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	result := &user{
		Name_:        valid["name"].(string),
		DisplayName_: valid["display-name"].(string),
		CreatedBy_:   valid["created-by"].(string),
		DateCreated_: valid["date-created"].(time.Time),
		Access_:      valid["access"].(string),
	}

	lastConn := valid["last-connection"].(time.Time)
	if !lastConn.IsZero() {
		result.LastConnection_ = &lastConn
	}

	return result, nil

}
