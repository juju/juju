// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
)

type stubStatePersistence struct {
	stub *testing.Stub

	docs []resourceDoc
}

func (s stubStatePersistence) All(collName string, query, docs interface{}) error {
	s.stub.AddCall("All", collName, query, docs)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	actual := docs.(*[]resourceDoc)
	*actual = s.docs
	return nil
}
