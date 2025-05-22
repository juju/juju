// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type typesSuite struct {
	testhelpers.IsolationSuite
}

func TestTypesSuite(t *stdtesting.T) {
	tc.Run(t, &typesSuite{})
}

func ptr[T any](x T) *T {
	return &x
}

func (s *typesSuite) TestToSecretBackends(c *tc.C) {
	rows := secretBackendRows{
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
	c.Assert(result, tc.DeepEquals, []*secretbackend.SecretBackend{
		{
			ID:                  "uuid1",
			Name:                "name1",
			BackendType:         "vault",
			TokenRotateInterval: ptr(10 * time.Second),
			Config: map[string]any{
				"config11": "content11",
				"config12": "content12",
				"config13": "content13",
			},
		},
		{
			ID:          "uuid2",
			Name:        "name2",
			BackendType: "vault",
			Config: map[string]any{
				"config21": "content21",
			},
		},
		{
			ID:                  "uuid3",
			Name:                "name3",
			BackendType:         "vault",
			TokenRotateInterval: ptr(30 * time.Second),
			Config: map[string]any{
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
			Config: map[string]any{
				"config51": "content51",
				"config52": "content52",
			},
		},
	})
}

func (s *typesSuite) TestToChanges(c *tc.C) {
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
	result := rows.toChanges(loggertesting.WrapCheckLog(c))
	c.Assert(result, tc.DeepEquals, []watcher.SecretBackendRotateChange{
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
