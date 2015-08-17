// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"
)

// token implements leadership.Token.
type token struct {
	serviceName string
	unitName    string
	checks      chan<- check
	abort       <-chan struct{}
}

// Check is part of the leadership.Token interface.
func (t token) Check(out interface{}) error {

	// Check validity and get the assert op in case it's needed.
	op, err := check{
		serviceName: t.serviceName,
		unitName:    t.unitName,
		response:    make(chan txn.Op),
		abort:       t.abort,
	}.invoke(t.checks)
	if err != nil {
		return errors.Trace(err)
	}

	// Report transaction ops if the client wants them.
	if out != nil {
		outPtr, ok := out.(*[]txn.Op)
		if !ok {
			return errors.New("expected pointer to []txn.Op")
		}
		*outPtr = []txn.Op{op}
	}
	return nil
}
