// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package auth provides common types and functions for aiding in authentication
// within Juju. Currently this package provides our password logic for hashing
// and encapsulating plain text passwords.
//
// When a component in Juju receives a plain text password from a user it should
// be immediately wrapped in a Password type with NewPassword("mypassword").
//
// To hash a new password for Juju to persist the first step is to generate a
// new password salt with NewSalt(). This newly created salt value must follow
// the users password for the life of the password.
//
// Passwords can be hashed with HashPassword(password, salt). The resultant hash
// is now safe for storing in Juju along with the created salt.
package auth
