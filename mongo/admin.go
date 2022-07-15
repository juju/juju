// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"fmt"

	"github.com/juju/mgo/v3"
)

// AdminUser is the name of the user that is initially created in mongo.
const AdminUser = "admin"

// SetAdminMongoPassword sets the administrative password
// to access a mongo database. If the password is non-empty,
// all subsequent attempts to access the database must
// be authorized; otherwise no authorization is required.
func SetAdminMongoPassword(session *mgo.Session, user, password string) error {
	admin := session.DB("admin")
	if password != "" {
		if err := admin.UpsertUser(&mgo.User{
			Username: user,
			Password: password,
			Roles:    []mgo.Role{mgo.RoleDBAdminAny, mgo.RoleUserAdminAny, mgo.RoleClusterAdmin, mgo.RoleReadWriteAny},
		}); err != nil {
			return fmt.Errorf("cannot set admin password: %v", err)
		}
	} else {
		if err := admin.RemoveUser(user); err != nil && err != mgo.ErrNotFound {
			return fmt.Errorf("cannot disable admin password: %v", err)
		}
	}
	return nil
}
