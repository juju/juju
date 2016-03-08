// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

// epoch is the time we use for "since" in statuses. The time
// is always shown as a local time, so we override the local
// location to be UTC+8.
var epoch = time.Unix(0, 0)

type ShowSuite struct {
	SubStorageSuite
	mockAPI *mockShowAPI
}

var _ = gc.Suite(&ShowSuite{})

func (s *ShowSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockShowAPI{}
	s.PatchValue(&time.Local, time.FixedZone("Australia/Perth", 3600*8))
}

func (s *ShowSuite) runShow(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, storage.NewShowCommand(s.mockAPI, s.store), args...)
}

func (s *ShowSuite) TestShowNoMatch(c *gc.C) {
	s.mockAPI.noMatch = true
	s.assertValidShow(
		c,
		[]string{"fluff/0"},
		`
{}
`[1:],
	)
}

func (s *ShowSuite) TestShow(c *gc.C) {
	s.assertValidShow(
		c,
		[]string{"shared-fs/0"},
		// Default format is yaml
		`
shared-fs/0:
  kind: filesystem
  status:
    current: attached
    since: 01 Jan 1970 08:00:00\+08:00
  persistent: true
  attachments:
    units:
      transcode/0:
        machine: \"1\"
        location: a location
      transcode/1:
        machine: \"2\"
        location: b location
`[1:],
	)
}

func (s *ShowSuite) TestShowInvalidId(c *gc.C) {
	_, err := s.runShow(c, []string{"foo"})
	c.Assert(err, gc.ErrorMatches, ".*invalid storage id foo.*")
}

func (s *ShowSuite) TestShowJSON(c *gc.C) {
	s.assertValidShow(
		c,
		[]string{"shared-fs/0", "--format", "json"},
		`{"shared-fs/0":{"kind":"filesystem","status":{"current":"attached","since":"01 Jan 1970 08:00:00\+08:00"},"persistent":true,"attachments":{"units":{"transcode/0":{"machine":"1","location":"a location"},"transcode/1":{"machine":"2","location":"b location"}}}}}
`,
	)
}

func (s *ShowSuite) TestShowMultipleReturn(c *gc.C) {
	s.assertValidShow(
		c,
		[]string{"shared-fs/0", "db-dir/1000"},
		`
db-dir/1000:
  kind: block
  status:
    current: pending
    since: .*
  persistent: true
  attachments:
    units:
      postgresql/0: {}
shared-fs/0:
  kind: filesystem
  status:
    current: attached
    since: 01 Jan 1970 08:00:00\+08:00
  persistent: true
  attachments:
    units:
      transcode/0:
        machine: \"1\"
        location: a location
      transcode/1:
        machine: \"2\"
        location: b location
`[1:],
	)
}

func (s *ShowSuite) assertValidShow(c *gc.C, args []string, expected string) {
	context, err := s.runShow(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtained := testing.Stdout(context)
	c.Assert(obtained, gc.Matches, expected)
}

type mockShowAPI struct {
	noMatch bool
}

func (s mockShowAPI) Close() error {
	return nil
}

func (s mockShowAPI) StorageDetails(tags []names.StorageTag) ([]params.StorageDetailsResult, error) {
	if s.noMatch {
		return nil, nil
	}
	all := make([]params.StorageDetailsResult, len(tags))
	for i, tag := range tags {
		if strings.Contains(tag.String(), "shared") {
			all[i].Result = &params.StorageDetails{
				StorageTag: tag.String(),
				OwnerTag:   "service-transcode",
				Kind:       params.StorageKindFilesystem,
				Status: params.EntityStatus{
					Status: "attached",
					Since:  &epoch,
				},
				Persistent: true,
				Attachments: map[string]params.StorageAttachmentDetails{
					"unit-transcode-0": params.StorageAttachmentDetails{
						MachineTag: "machine-1",
						Location:   "a location",
					},
					"unit-transcode-1": params.StorageAttachmentDetails{
						MachineTag: "machine-2",
						Location:   "b location",
					},
				},
			}
		} else {
			all[i].Result = &params.StorageDetails{
				StorageTag: tag.String(),
				Kind:       params.StorageKindBlock,
				Status: params.EntityStatus{
					Status: "pending",
					Since:  &epoch,
				},
				Attachments: map[string]params.StorageAttachmentDetails{
					"unit-postgresql-0": params.StorageAttachmentDetails{},
				},
			}
			if i == 1 {
				all[i].Result.Persistent = true
			}
		}
	}
	return all, nil
}
