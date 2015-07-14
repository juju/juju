// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"
)

// token implements leadership.Token.
type token struct {
	op txn.Op
}

// Read is part of the leadership.Token interface.
func (t token) Read(out interface{}) error {
	outPtr, ok := out.(*[]txn.Op)
	if !ok {
		return errors.New("expected pointer to []txn.Op")
	}
	*outPtr = []txn.Op{t.op}
	return nil
}
