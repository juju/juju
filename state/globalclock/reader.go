// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
)

// Reader provides a means of reading the global clock time.
//
// Reader is not goroutine-safe.
type Reader struct {
	config ReaderConfig
}

// NewReader returns a new Reader using the supplied config, or an error.
//
// Readers will not function past the lifetime of their configured Mongo.
func NewReader(config ReaderConfig) (*Reader, error) {
	if err := config.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	r := &Reader{config: config}
	return r, nil
}

// Now returns the current global time.
func (r *Reader) Now() (time.Time, error) {
	coll, closer := r.config.Mongo.GetCollection(r.config.Collection)
	defer closer()

	t, err := readClock(coll)
	if errors.Cause(err) == mgo.ErrNotFound {
		// No time written yet. When it is written
		// for the first time, it'll be globalEpoch.
		t = globalEpoch
	} else if err != nil {
		return time.Time{}, errors.Trace(err)
	}
	return t, nil
}
