// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/life"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&storageSuite{})

type storageSuite struct {
	coretesting.BaseSuite
}

func (s *storageSuite) TestUnitStorageAttachments(c *tc.C) {
	storageAttachmentIds := []params.StorageAttachmentId{{
		StorageTag: "storage-whatever-0",
		UnitTag:    "unit-mysql-0",
	}}

	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Uniter")
		c.Check(version, tc.Equals, 2)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "UnitStorageAttachments")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-0"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StorageAttachmentIdsResults{})
		*(result.(*params.StorageAttachmentIdsResults)) = params.StorageAttachmentIdsResults{
			Results: []params.StorageAttachmentIdsResult{{
				Result: params.StorageAttachmentIds{storageAttachmentIds},
			}},
		}
		called = true
		return nil
	})

	caller := testing.BestVersionCaller{apiCaller, 2}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))
	attachmentIds, err := client.UnitStorageAttachments(context.Background(), names.NewUnitTag("mysql/0"))
	c.Check(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
	c.Assert(attachmentIds, tc.DeepEquals, storageAttachmentIds)
}

func (s *storageSuite) TestDestroyUnitStorageAttachments(c *tc.C) {
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Uniter")
		c.Check(version, tc.Equals, 2)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "DestroyUnitStorageAttachments")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-0"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		called = true
		return nil
	})

	caller := testing.BestVersionCaller{apiCaller, 2}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))
	err := client.DestroyUnitStorageAttachments(context.Background(), names.NewUnitTag("mysql/0"))
	c.Check(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
}

func (s *storageSuite) TestStorageAttachmentResultCountMismatch(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.StorageAttachmentIdsResults)) = params.StorageAttachmentIdsResults{
			[]params.StorageAttachmentIdsResult{{}, {}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))
	_, err := client.UnitStorageAttachments(context.Background(), names.NewUnitTag("mysql/0"))
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}

func (s *storageSuite) TestAPIErrors(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("bad")
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))
	_, err := client.UnitStorageAttachments(context.Background(), names.NewUnitTag("mysql/0"))
	c.Check(err, tc.ErrorMatches, "bad")
}

func (s *storageSuite) TestWatchUnitStorageAttachments(c *tc.C) {
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Uniter")
		c.Check(version, tc.Equals, 2)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchUnitStorageAttachments")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-0"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		called = true
		return nil
	})

	caller := testing.BestVersionCaller{apiCaller, 2}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))
	_, err := client.WatchUnitStorageAttachments(context.Background(), names.NewUnitTag("mysql/0"))
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(called, tc.IsTrue)
}

func (s *storageSuite) TestWatchStorageAttachments(c *tc.C) {
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Uniter")
		c.Check(version, tc.Equals, 2)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchStorageAttachments")
		c.Check(arg, tc.DeepEquals, params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{{
				StorageTag: "storage-data-0",
				UnitTag:    "unit-mysql-0",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		called = true
		return nil
	})

	caller := testing.BestVersionCaller{apiCaller, 2}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))
	_, err := client.WatchStorageAttachment(context.Background(), names.NewStorageTag("data/0"), names.NewUnitTag("mysql/0"))
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(called, tc.IsTrue)
}

func (s *storageSuite) TestStorageAttachments(c *tc.C) {
	storageAttachment := params.StorageAttachment{
		StorageTag: "storage-whatever-0",
		OwnerTag:   "application-mysql",
		UnitTag:    "unit-mysql-0",
		Kind:       params.StorageKindBlock,
		Location:   "/dev/sda",
	}

	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Uniter")
		c.Check(version, tc.Equals, 2)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "StorageAttachments")
		c.Check(arg, tc.DeepEquals, params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{{
				StorageTag: "storage-data-0",
				UnitTag:    "unit-mysql-0",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StorageAttachmentResults{})
		*(result.(*params.StorageAttachmentResults)) = params.StorageAttachmentResults{
			Results: []params.StorageAttachmentResult{{
				Result: storageAttachment,
			}},
		}
		called = true
		return nil
	})

	caller := testing.BestVersionCaller{apiCaller, 2}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))
	attachment, err := client.StorageAttachment(context.Background(), names.NewStorageTag("data/0"), names.NewUnitTag("mysql/0"))
	c.Check(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
	c.Assert(attachment, tc.DeepEquals, storageAttachment)
}

func (s *storageSuite) TestStorageAttachmentLife(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Uniter")
		c.Check(version, tc.Equals, 2)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "StorageAttachmentLife")
		c.Check(arg, tc.DeepEquals, params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{{
				StorageTag: "storage-data-0",
				UnitTag:    "unit-mysql-0",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Life: life.Dying,
			}},
		}
		return nil
	})

	caller := testing.BestVersionCaller{apiCaller, 2}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))
	results, err := client.StorageAttachmentLife(context.Background(), []params.StorageAttachmentId{{
		StorageTag: "storage-data-0",
		UnitTag:    "unit-mysql-0",
	}})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []params.LifeResult{{Life: life.Dying}})
}

func (s *storageSuite) TestRemoveStorageAttachment(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Uniter")
		c.Check(version, tc.Equals, 2)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "RemoveStorageAttachments")
		c.Check(arg, tc.DeepEquals, params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{{
				StorageTag: "storage-data-0",
				UnitTag:    "unit-mysql-0",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "yoink"},
			}},
		}
		return nil
	})

	caller := testing.BestVersionCaller{apiCaller, 2}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))
	err := client.RemoveStorageAttachment(context.Background(), names.NewStorageTag("data/0"), names.NewUnitTag("mysql/0"))
	c.Check(err, tc.ErrorMatches, "yoink")
}
