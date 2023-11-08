// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"
	"time"
)

type User struct {
	CreatedAt   time.Time
	DisplayName string
	Name        string
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
