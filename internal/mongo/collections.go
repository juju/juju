// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

// Iterator defines the parts of the mgo.Iter that we use - this
// interface allows us to switch out the querying for testing.
type Iterator interface {
	Next(interface{}) bool
	Timeout() bool
	Close() error
}
