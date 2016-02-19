// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"
)

var _ = gc.Suite(&StagedResourceSuite{})

type StagedResourceSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
	base *stubStatePersistence
}

func (s *StagedResourceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.base = &stubStatePersistence{
		stub: s.stub,
	}
}

func (s *StagedResourceSuite) newStagedResource(c *gc.C, serviceID, name string) (*StagedResource, resourceDoc) {
	stored, doc := newResource(c, serviceID, name)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, ignoredErr)
	staged := &StagedResource{
		base:   s.base,
		id:     stored.ID,
		stored: stored,
	}
	return staged, doc
}

func (s *StagedResourceSuite) TestStageOkay(c *gc.C) {
	staged, doc := s.newStagedResource(c, "a-service", "spam")
	doc.DocID += "#staged"
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, ignoredErr)

	err := staged.stage()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam#staged",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
}

func (s *StagedResourceSuite) TestStageExists(c *gc.C) {
	staged, doc := s.newStagedResource(c, "a-service", "spam")
	doc.DocID += "#staged"
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, txn.ErrAborted, nil, ignoredErr)

	err := staged.stage()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam#staged",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam#staged",
		Assert: &doc,
	}})
}

func (s *StagedResourceSuite) TestUnstageOkay(c *gc.C) {
	staged, _ := s.newStagedResource(c, "a-service", "spam")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, ignoredErr)

	err := staged.Unstage()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam#staged",
		Remove: true,
	}})
}

func (s *StagedResourceSuite) TestActivateOkay(c *gc.C) {
	staged, doc := s.newStagedResource(c, "a-service", "spam")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, ignoredErr)

	err := staged.Activate()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "IncCharmModifiedVersionOps", "RunTransaction")
	s.stub.CheckCall(c, 1, "IncCharmModifiedVersionOps", "a-service")
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "resources",
		Id:     "resource#a-service/spam#staged",
		Remove: true,
	}})
}

func (s *StagedResourceSuite) TestActivateExists(c *gc.C) {
	staged, doc := s.newStagedResource(c, "a-service", "spam")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, txn.ErrAborted, nil, ignoredErr)

	err := staged.Activate()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "IncCharmModifiedVersionOps", "RunTransaction", "IncCharmModifiedVersionOps", "RunTransaction")
	s.stub.CheckCall(c, 1, "IncCharmModifiedVersionOps", "a-service")
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "resources",
		Id:     "resource#a-service/spam#staged",
		Remove: true,
	}})
	s.stub.CheckCall(c, 3, "IncCharmModifiedVersionOps", "a-service")
	s.stub.CheckCall(c, 4, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam",
		Assert: txn.DocExists,
		Remove: true,
	}, {
		C:      "resources",
		Id:     "resource#a-service/spam",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "resources",
		Id:     "resource#a-service/spam#staged",
		Remove: true,
	}})
}
