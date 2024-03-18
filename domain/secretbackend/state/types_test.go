// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	jujutesting "github.com/juju/juju/testing"
)

type typesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typesSuite{})

func ptr[T any](x T) *T {
	return &x
}

func (s *typesSuite) TestToSecretBackendInfo(c *gc.C) {
	rows := SecretBackendRows{
		{
			UUID:        "uuid1",
			Name:        "name1",
			BackendType: "vault",
			TokenRotateInterval: domain.NullableDuration{
				Duration: 10 * time.Second,
				Valid:    true,
			},
			ConfigName:    "config11",
			ConfigContent: "content11",
		},
		{
			UUID:        "uuid1",
			Name:        "name1",
			BackendType: "vault",
			TokenRotateInterval: domain.NullableDuration{
				Duration: 10 * time.Second,
				Valid:    true,
			},
			ConfigName:    "config12",
			ConfigContent: "content12",
		},
		{
			UUID:        "uuid2",
			Name:        "name2",
			BackendType: "vault",
			TokenRotateInterval: domain.NullableDuration{
				Valid: false,
			},
			ConfigName:    "config21",
			ConfigContent: "content21",
		},
		{
			UUID:        "uuid3",
			Name:        "name3",
			BackendType: "vault",
			TokenRotateInterval: domain.NullableDuration{
				Duration: 30 * time.Second,
				Valid:    true,
			},
			ConfigName:    "config31",
			ConfigContent: "content31",
		},
		{
			UUID:        "uuid1",
			Name:        "name1",
			BackendType: "vault",
			TokenRotateInterval: domain.NullableDuration{
				Duration: 10 * time.Second,
				Valid:    true,
			},
			ConfigName:    "config13",
			ConfigContent: "content13",
		},
		{
			UUID:        "uuid4",
			Name:        "name4",
			BackendType: "vault",
		},
	}
	result := rows.ToSecretBackendInfo()
	c.Assert(result, gc.DeepEquals, []*coresecrets.SecretBackend{
		{
			ID:                  "uuid1",
			Name:                "name1",
			BackendType:         "vault",
			TokenRotateInterval: ptr(10 * time.Second),
			Config: map[string]interface{}{
				"config11": "content11",
				"config12": "content12",
				"config13": "content13",
			},
		},
		{
			ID:          "uuid2",
			Name:        "name2",
			BackendType: "vault",
			Config: map[string]interface{}{
				"config21": "content21",
			},
		},
		{
			ID:                  "uuid3",
			Name:                "name3",
			BackendType:         "vault",
			TokenRotateInterval: ptr(30 * time.Second),
			Config: map[string]interface{}{
				"config31": "content31",
			},
		},
		{
			ID:          "uuid4",
			Name:        "name4",
			BackendType: "vault",
		},
	})
}

func (s *typesSuite) TestToChanges(c *gc.C) {
	now := time.Now()
	rows := SecretBackendRotationRows{
		{
			UUID:             "uuid1",
			Name:             "name1",
			NextRotationTime: sql.NullTime{Time: now.Add(1 * time.Second), Valid: true},
		},
		{
			UUID:             "uuid2",
			Name:             "name2",
			NextRotationTime: sql.NullTime{Time: now.Add(2 * time.Second), Valid: true},
		},
		{
			UUID:             "uuid3",
			Name:             "name3",
			NextRotationTime: sql.NullTime{Valid: false},
		},
	}
	result := rows.ToChanges(jujutesting.NewCheckLogger(c))
	c.Assert(result, gc.DeepEquals, []watcher.SecretBackendRotateChange{
		{
			ID:              "uuid1",
			Name:            "name1",
			NextTriggerTime: now.Add(1 * time.Second),
		},
		{
			ID:              "uuid2",
			Name:            "name2",
			NextTriggerTime: now.Add(2 * time.Second),
		},
	})
}
