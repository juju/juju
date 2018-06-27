// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gomockmatcher

import (
	"fmt"

	"github.com/golang/mock/gomock"
)

type BoolType struct{ b bool }

func OfTypeBool(b bool) gomock.Matcher {
	return &BoolType{b}
}

func (o *BoolType) Matches(x interface{}) bool {
	b, ok := x.(bool)
	if !ok {
		return false
	}
	return o.b == b
}

func (o *BoolType) String() string {
	return fmt.Sprintf("is bool equal %t", o.b)
}
