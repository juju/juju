// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"
	"time"
)

// User describes a user within Juju.
type User struct {
	// CreatedAt is the time that the user was created at.
	CreatedAt time.Time

	// DisplayName is a user friendly name represent the user as.
	DisplayName string

	// Name is the username of the user.
	Name string
}

// UserPassword type attempts to protect it against printouts
// because we get the password as a plain text over the wire
type UserPassword struct {
	password string
}

func NewUserPassword(password string) UserPassword {
	return UserPassword{
		password: password,
	}
}

func (up UserPassword) String() string {
	return ""
}

// Format implements the Formatter interface from fmt
func (up UserPassword) Format(f fmt.State, verb rune) {
}

// GoString implements the GoStringer interface from fmt
func (up UserPassword) GoString() string {
	return ""
}
