// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&storageSuite{})

type storageSuite struct {
	coretesting.BaseSuite
}

func (s *storageSuite) TestStorageAttachments(c *gc.C) {
	storageAttachments := []params.StorageAttachment{{
		StorageTag: "storage-whatever-0",
		OwnerTag:   "service-mysql",
		UnitTag:    "unit-mysql-0",
		Kind:       params.StorageKindBlock,
		Location:   "/dev/sda",
	}}

	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Uniter")
		c.Check(version, gc.Equals, 2)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "StorageAttachments")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-0"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StorageAttachmentsResults{})
		*(result.(*params.StorageAttachmentsResults)) = params.StorageAttachmentsResults{
			Results: []params.StorageAttachmentsResult{{
				Result: storageAttachments,
			}},
		}
		called = true
		return nil
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	attachments, err := st.StorageAttachments(names.NewUnitTag("mysql/0"))
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
	c.Assert(attachments, gc.DeepEquals, storageAttachments)
}

func (s *storageSuite) TestStorageAttachmentResultCountMismatch(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.StorageAttachmentsResults)) = params.StorageAttachmentsResults{
			[]params.StorageAttachmentsResult{{}, {}},
		}
		return nil
	})
	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	c.Assert(func() { st.StorageAttachments(names.NewUnitTag("mysql/0")) }, gc.PanicMatches, "expected 1 result, got 2")
}

func (s *storageSuite) TestAPIErrors(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("bad")
	})
	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.StorageAttachments(names.NewUnitTag("mysql/0"))
	c.Check(err, gc.ErrorMatches, "bad")
}
