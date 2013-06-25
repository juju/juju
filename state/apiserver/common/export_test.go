// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

func NewMockPasswordChanger(st authGetter, canChange func(tag string) bool) *PasswordChanger {
	return &PasswordChanger{
		st:        st,
		canChange: canChange,
	}
}
