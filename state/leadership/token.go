// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"
)

// token implements leadership.Token.
type token struct {
	leaseName  string
	holderName string
	secretary  Secretary
	checks     chan<- check
	abort      <-chan struct{}
}

// Check is part of the leadership.Token interface.
func (t token) Check(out interface{}) error {

	// Check validity and get the assert op in case it's needed.
	op, err := check{
		leaseName:  t.leaseName,
		holderName: t.holderName,
		response:   make(chan txn.Op),
		abort:      t.abort,
	}.invoke(
		t.secretary,
		t.checks,
	)
	if err != nil {
		return errors.Trace(err)
	}

	// Report transaction ops if the client wants them.
	if out != nil {
		outPtr, ok := out.(*[]txn.Op)
		if !ok {
			return errors.NotValidf("expected *[]txn.Op; %T", out)
		}
		*outPtr = []txn.Op{op}
	}
	return nil
}
