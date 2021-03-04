// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/statetest"
)

var _ = gc.Suite(&StagedResourceSuite{})

type StagedResourceSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
	base *statetest.StubPersistence
}

func (s *StagedResourceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.base = statetest.NewStubPersistence(s.stub)
	s.base.ReturnApplicationExistsOps = []txn.Op{{
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}}
}

func (s *StagedResourceSuite) newStagedResource(c *gc.C, applicationID, name string) (*StagedResource, resourceDoc) {
	stored, doc := newPersistenceResource(c, applicationID, name)
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
	staged, doc := s.newStagedResource(c, "a-application", "spam")
	doc.DocID += "#staged"
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, ignoredErr)

	err := staged.stage()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "ApplicationExistsOps", "RunTransaction")
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam#staged",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}})
}

func (s *StagedResourceSuite) TestStageExists(c *gc.C) {
	staged, doc := s.newStagedResource(c, "a-application", "spam")
	doc.DocID += "#staged"
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, txn.ErrAborted, nil, nil, ignoredErr)

	err := staged.stage()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "ApplicationExistsOps", "RunTransaction", "ApplicationExistsOps", "RunTransaction")
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam#staged",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}})
	s.stub.CheckCall(c, 4, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam#staged",
		Assert: &doc,
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}})
}

func (s *StagedResourceSuite) TestUnstageOkay(c *gc.C) {
	staged, _ := s.newStagedResource(c, "a-application", "spam")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, ignoredErr)

	err := staged.Unstage()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam#staged",
		Remove: true,
	}})
}

func (s *StagedResourceSuite) TestActivateOkay(c *gc.C) {
	staged, doc := s.newStagedResource(c, "a-application", "spam")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, nil, nil, ignoredErr)

	err := staged.Activate()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "ApplicationExistsOps", "One", "IncCharmModifiedVersionOps", "RunTransaction")
	s.stub.CheckCall(c, 3, "IncCharmModifiedVersionOps", "a-application")
	s.stub.CheckCall(c, 4, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}, {
		C:      "resources",
		Id:     "resource#a-application/spam#staged",
		Remove: true,
	}})
}

func (s *StagedResourceSuite) TestActivateExists(c *gc.C) {
	staged, doc := s.newStagedResource(c, "a-application", "spam")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, nil, txn.ErrAborted, nil, nil, nil, nil, ignoredErr)

	err := staged.Activate()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "ApplicationExistsOps", "One", "IncCharmModifiedVersionOps", "RunTransaction", "ApplicationExistsOps", "One", "IncCharmModifiedVersionOps", "RunTransaction")
	s.stub.CheckCall(c, 3, "IncCharmModifiedVersionOps", "a-application")
	s.stub.CheckCall(c, 4, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}, {
		C:      "resources",
		Id:     "resource#a-application/spam#staged",
		Remove: true,
	}})
	s.stub.CheckCall(c, 7, "IncCharmModifiedVersionOps", "a-application")
	s.stub.CheckCall(c, 8, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam",
		Assert: txn.DocExists,
		Update: bson.M{"$set": bson.M{
			"resource-id":                doc.ID,
			"pending-id":                 doc.PendingID,
			"application-id":             doc.ApplicationID,
			"unit-id":                    doc.UnitID,
			"name":                       doc.Name,
			"type":                       doc.Type,
			"path":                       doc.Path,
			"description":                doc.Description,
			"origin":                     doc.Origin,
			"revision":                   doc.Revision,
			"fingerprint":                doc.Fingerprint,
			"size":                       doc.Size,
			"username":                   doc.Username,
			"timestamp-when-added":       doc.Timestamp,
			"storage-path":               doc.StoragePath,
			"download-progress":          doc.DownloadProgress,
			"timestamp-when-last-polled": doc.LastPolled,
		}},
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}, {
		C:      "resources",
		Id:     "resource#a-application/spam#staged",
		Remove: true,
	}})
}
