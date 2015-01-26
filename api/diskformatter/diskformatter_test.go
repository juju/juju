// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/diskformatter"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/names"
)

var _ = gc.Suite(&DiskFormatterSuite{})

type DiskFormatterSuite struct {
	coretesting.BaseSuite
}

func (s *DiskFormatterSuite) TestAttachedVolumes(c *gc.C) {
	attachments := []params.VolumeAttachment{{}}

	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "DiskFormatter")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "AttachedVolumes")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-0"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.VolumeAttachmentsResults{})
		*(result.(*params.VolumeAttachmentsResults)) = params.VolumeAttachmentsResults{
			Results: []params.VolumeAttachmentsResult{{
				Attachments: attachments,
			}},
		}
		called = true
		return nil
	})

	st := diskformatter.NewState(apiCaller, names.NewMachineTag("0"))
	result, err := st.AttachedVolumes()
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
	c.Assert(result, gc.DeepEquals, attachments)
}

func (s *DiskFormatterSuite) TestBlockDeviceResultCountMismatch(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.VolumeAttachmentsResults)) = params.VolumeAttachmentsResults{
			Results: []params.VolumeAttachmentsResult{{}, {}},
		}
		return nil
	})
	st := diskformatter.NewState(apiCaller, names.NewMachineTag("0"))
	c.Assert(func() { st.AttachedVolumes() }, gc.PanicMatches, "expected 1 result, got 2")
}

func (s *DiskFormatterSuite) TestVolumeFormattingInfo(c *gc.C) {
	expected := []params.VolumeFormattingInfoResult{{
		Result: params.VolumeFormattingInfo{
			DevicePath:      "/dev/sdx",
			NeedsFormatting: true,
		},
	}, {
		Error: &params.Error{Message: "MSG", Code: "621"},
	}}

	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "DiskFormatter")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "VolumeFormattingInfo")
		c.Check(arg, gc.DeepEquals, params.VolumeAttachmentIds{
			Ids: []params.VolumeAttachmentId{
				{MachineTag: "machine-0", VolumeTag: "disk-0"},
				{MachineTag: "machine-0", VolumeTag: "disk-1"},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.VolumeFormattingInfoResults{})
		*(result.(*params.VolumeFormattingInfoResults)) = params.VolumeFormattingInfoResults{
			expected,
		}
		called = true
		return nil
	})

	st := diskformatter.NewState(apiCaller, names.NewMachineTag("0"))
	results, err := st.VolumeFormattingInfo([]names.DiskTag{
		names.NewDiskTag("0"),
		names.NewDiskTag("1"),
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
	c.Assert(results, gc.DeepEquals, expected)
}

func (s *DiskFormatterSuite) TestAPIErrors(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("blargh")
	})
	st := diskformatter.NewState(apiCaller, names.NewMachineTag("0"))
	_, err := st.AttachedVolumes()
	c.Check(err, gc.ErrorMatches, "blargh")
	_, err = st.VolumeFormattingInfo(nil)
	c.Check(err, gc.ErrorMatches, "blargh")
}
