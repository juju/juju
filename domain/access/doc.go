// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package access provides the services for managing users and permissions in
// Juju. The users side is mainly concerned with authentication and the
// permissions side with authorization.
//
// External users (i.e. users with an external domain in their user name e.g.
// someone@external) can inherit permissions from the everyone@external user.
//
// The permissions for everyone@external act like a group for external users.
// When any external users permissions are read the users access level will be
// compared against the access level of everyone@external. If everyone@external
// user has a higher access level then the external user will inherit the higher
// level. This check is performed every time an external users permissions are
// read.
//
// If an external user inherits its login permission from everyone@external then
// it should be created as a user in the database on login.
package access
