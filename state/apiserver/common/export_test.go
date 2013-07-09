// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

func NewMockPasswordChanger(st authGetter, getCanChange GetAuthFunc) *PasswordChanger {
	return &PasswordChanger{
		st:           st,
		getCanChange: getCanChange,
	}
}

func NewMockLifeGetter(st liferGetter, getCanRead GetAuthFunc) *LifeGetter {
	return &LifeGetter{
		st:         st,
		getCanRead: getCanRead,
	}
}
