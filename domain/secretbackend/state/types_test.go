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
	"github.com/juju/juju/internal/database"
	jujutesting "github.com/juju/juju/testing"
)

type typesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typesSuite{})

func ptr[T any](x T) *T {
	return &x
}

func (s *typesSuite) TestToSecretBackends(c *gc.C) {
	rows := SecretBackendRows{
		{
			ID:          "uuid1",
			Name:        "name1",
			BackendType: "vault",
			TokenRotateInterval: database.NullDuration{
				Duration: 10 * time.Second,
				Valid:    true,
			},
			ConfigName:    "config11",
			ConfigContent: "content11",
		},
		{
			ID:          "uuid1",
			Name:        "name1",
			BackendType: "vault",
			TokenRotateInterval: database.NullDuration{
				Duration: 10 * time.Second,
				Valid:    true,
			},
			ConfigName:    "config12",
			ConfigContent: "content12",
		},
		{
			ID:          "uuid2",
			Name:        "name2",
			BackendType: "vault",
			TokenRotateInterval: database.NullDuration{
				Valid: false,
			},
			ConfigName:    "config21",
			ConfigContent: "content21",
		},
		{
			ID:          "uuid3",
			Name:        "name3",
			BackendType: "vault",
			TokenRotateInterval: database.NullDuration{
				Duration: 30 * time.Second,
				Valid:    true,
			},
			ConfigName:    "config31",
			ConfigContent: "content31",
		},
		{
			ID:          "uuid1",
			Name:        "name1",
			BackendType: "vault",
			TokenRotateInterval: database.NullDuration{
				Duration: 10 * time.Second,
				Valid:    true,
			},
			ConfigName:    "config13",
			ConfigContent: "content13",
		},
		{
			ID:          "uuid4",
			Name:        "name4",
			BackendType: "vault",
		},
		{
			ID:            "uuid5",
			Name:          "name5",
			BackendType:   "vault",
			ConfigName:    "config51",
			ConfigContent: "content51",
		},
		{
			ID:            "uuid5",
			Name:          "name5",
			BackendType:   "vault",
			ConfigName:    "config52",
			ConfigContent: "content52",
		},
	}
	result := rows.toSecretBackends()
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
		{
			ID:          "uuid5",
			Name:        "name5",
			BackendType: "vault",
			Config: map[string]interface{}{
				"config51": "content51",
				"config52": "content52",
			},
		},
	})
}

func (s *typesSuite) TestToChanges(c *gc.C) {
	now := time.Now()
	rows := SecretBackendRotationRows{
		{
			ID:               "uuid1",
			Name:             "name1",
			NextRotationTime: sql.NullTime{Time: now.Add(1 * time.Second), Valid: true},
		},
		{
			ID:               "uuid2",
			Name:             "name2",
			NextRotationTime: sql.NullTime{Time: now.Add(2 * time.Second), Valid: true},
		},
		{
			ID:               "uuid3",
			Name:             "name3",
			NextRotationTime: sql.NullTime{Valid: false},
		},
	}
	result := rows.toChanges(jujutesting.NewCheckLogger(c))
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
