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
	return o.b == x
}

func (o *BoolType) String() string {
	return fmt.Sprintf("is equal %t", o.b)
}
