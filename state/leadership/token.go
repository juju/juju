// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"gopkg.in/mgo.v2/txn"
)

// token implements Token.
type token struct {
	op txn.Op
}

// AssertOps is part of the Token interface.
func (t token) AssertOps() []txn.Op {
	return []txn.Op{t.op}
}
