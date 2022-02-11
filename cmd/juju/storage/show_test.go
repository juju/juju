// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
)

type ShowSuite struct {
	SubStorageSuite
	mockAPI *mockShowAPI
}

var _ = gc.Suite(&ShowSuite{})

func (s *ShowSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockShowAPI{}
}

func (s *ShowSuite) runShow(c *gc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewShowCommandForTest(s.mockAPI, s.store), args...)
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
	now := time.Now()
	s.mockAPI.time = now
	s.assertValidShow(
		c,
		[]string{"shared-fs/0"},
		// Default format is yaml
		fmt.Sprintf(`
shared-fs/0:
  kind: filesystem
  status:
    current: attached
    since: %s
  persistent: true
  attachments:
    units:
      transcode/0:
        machine: "1"
        location: a location
      transcode/1:
        machine: "2"
        location: b location
`[1:], common.FormatTime(&now, false)),
	)
}

func (s *ShowSuite) TestShowInvalidId(c *gc.C) {
	_, err := s.runShow(c, []string{"foo"})
	c.Assert(err, gc.ErrorMatches, ".*invalid storage id foo.*")
}

func (s *ShowSuite) TestShowJSON(c *gc.C) {
	now := time.Now()
	s.mockAPI.time = now
	s.assertValidShow(
		c,
		[]string{"shared-fs/0", "--format", "json"},
		fmt.Sprintf(`{"shared-fs/0":{"kind":"filesystem","status":{"current":"attached","since":"%s"},"persistent":true,"attachments":{"units":{"transcode/0":{"machine":"1","location":"a location"},"transcode/1":{"machine":"2","location":"b location"}}}}}
`, common.FormatTime(&now, false)),
	)
}

func (s *ShowSuite) TestShowMultipleReturn(c *gc.C) {
	now := time.Now()
	s.mockAPI.time = now
	since := common.FormatTime(&now, false)

	s.assertValidShow(
		c,
		[]string{"shared-fs/0", "db-dir/1000"},
		fmt.Sprintf(`
db-dir/1000:
  kind: block
  status:
    current: pending
    since: %s
  persistent: true
  attachments:
    units:
      postgresql/0: {}
shared-fs/0:
  kind: filesystem
  status:
    current: attached
    since: %s
  persistent: true
  attachments:
    units:
      transcode/0:
        machine: "1"
        location: a location
      transcode/1:
        machine: "2"
        location: b location
`[1:], since, since),
	)
}

func (s *ShowSuite) assertValidShow(c *gc.C, args []string, expected string) {
	context, err := s.runShow(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtained := cmdtesting.Stdout(context)
	c.Assert(obtained, gc.Equals, expected)
}

type mockShowAPI struct {
	noMatch bool
	time    time.Time
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
				OwnerTag:   "application-transcode",
				Kind:       params.StorageKindFilesystem,
				Status: params.EntityStatus{
					Status: "attached",
					Since:  &s.time,
				},
				Persistent: true,
				Attachments: map[string]params.StorageAttachmentDetails{
					"unit-transcode-0": {
						MachineTag: "machine-1",
						Location:   "a location",
					},
					"unit-transcode-1": {
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
					Since:  &s.time,
				},
				Attachments: map[string]params.StorageAttachmentDetails{
					"unit-postgresql-0": {},
				},
			}
			if i == 1 {
				all[i].Result.Persistent = true
			}
		}
	}
	return all, nil
}
