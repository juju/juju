// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type typesSuite struct {
	testhelpers.IsolationSuite
}

func TestTypesSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
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
			ConfigContent: tc.Must1(c, encodeConfigValue, "content11"),
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
			ConfigContent: tc.Must1(c, encodeConfigValue, "content12"),
		},
		{
			ID:          "uuid2",
			Name:        "name2",
			BackendType: "vault",
			TokenRotateInterval: database.NullDuration{
				Valid: false,
			},
			ConfigName:    "config21",
			ConfigContent: tc.Must1(c, encodeConfigValue, "content21"),
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
			ConfigContent: tc.Must1(c, encodeConfigValue, "content31"),
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
			ConfigContent: tc.Must1(c, encodeConfigValue, "content13"),
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
			ConfigContent: tc.Must1(c, encodeConfigValue, "content51"),
		},
		{
			ID:            "uuid5",
			Name:          "name5",
			BackendType:   "vault",
			ConfigName:    "config52",
			ConfigContent: tc.Must1(c, encodeConfigValue, "content52"),
		},
		{
			ID:          "uuid6",
			Name:        "name6",
			BackendType: "vault",
			ConfigName:  "slice",
			ConfigContent: tc.Must1(c, encodeConfigValue, any([]any{`some
lines`, "some-other-lines"})),
		},
		{
			ID:          "uuid6",
			Name:        "name6",
			BackendType: "vault",
			ConfigName:  "map",
			ConfigContent: tc.Must1(c, encodeConfigValue, any(map[string]any{
				"key1": "value1",
				"key2": "value2",
			})),
		},
	}
	result := rows.toSecretBackends(c.Context(), loggertesting.WrapCheckLog(c))
	c.Assert(result, tc.DeepEquals, []*secretbackend.SecretBackend{
		{
			ID:                  "uuid1",
			Name:                "name1",
			BackendType:         "vault",
			TokenRotateInterval: new(10 * time.Second),
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
			TokenRotateInterval: new(30 * time.Second),
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
		{
			ID:          "uuid6",
			Name:        "name6",
			BackendType: "vault",
			Config: map[string]any{
				"slice": []any{"some\nlines", "some-other-lines"},
				"map": map[string]any{
					"key1": "value1",
					"key2": "value2",
				},
			},
		},
	})
}

func (s *typesSuite) TestToSecretBackendK8s(c *tc.C) {
	rows := secretBackendForK8sModelRows{
		{
			SecretBackendRow: SecretBackendRow{
				ID:          "uuid-k8s",
				Name:        "kubernetes",
				BackendType: "kubernetes",
			},
			ModelUUID:    "model1-uuid",
			ModelName:    "model1",
			CloudID:      "cloud1",
			CredentialID: "cred1",
		},
		{
			SecretBackendRow: SecretBackendRow{
				ID:          "uuid-k8s",
				Name:        "kubernetes",
				BackendType: "kubernetes",
			},
			ModelUUID:    "model2-uuid",
			ModelName:    "model2",
			CloudID:      "cloud1",
			CredentialID: "cred1",
		},
	}
	cldData := cloudRows{
		{
			ID:                "cloud1",
			Name:              "cloud1-name",
			Endpoint:          "https://cloud1.com",
			SkipTLSVerify:     true,
			IsControllerCloud: false,
			CACert:            "ca-cert1",
		},
	}
	credData := cloudCredentialRows{
		{
			ID:             "cred1",
			Name:           "cred1-name",
			AuthType:       "userpass",
			AttributeKey:   "token",
			AttributeValue: "token-val",
		},
	}

	result, err := rows.toSecretBackend("my-controller", cldData, credData)
	c.Assert(err, tc.IsNil)

	// Verify that we have 2 backends (one for each model), despite having the
	// same CloudID.
	c.Assert(result, tc.HasLen, 2)

	c.Assert(result[0].Name, tc.Equals, "model1-local")
	c.Assert(result[0].Config["namespace"], tc.Equals, "model1")

	c.Assert(result[1].Name, tc.Equals, "model2-local")
	c.Assert(result[1].Config["namespace"], tc.Equals, "model2")
}

// Test that when the k8s backend query yields multiple rows for the same
// model/credential (one per credential attribute key), we still only get
// a single SecretBackend per model.
func (s *typesSuite) TestToSecretBackendK8sDuplicateRows(c *tc.C) {
	rows := secretBackendForK8sModelRows{
		// model1 appears twice with the same credential, simulating a join
		// that yields one row per credential attribute for cred1
		{
			SecretBackendRow: SecretBackendRow{
				ID:          "uuid-k8s",
				Name:        "kubernetes",
				BackendType: "kubernetes",
			},
			ModelUUID:    "model1-uuid",
			ModelName:    "model1",
			CloudID:      "cloud1",
			CredentialID: "cred1",
		},
		{
			SecretBackendRow: SecretBackendRow{
				ID:          "uuid-k8s",
				Name:        "kubernetes",
				BackendType: "kubernetes",
			},
			ModelUUID:    "model1-uuid",
			ModelName:    "model1",
			CloudID:      "cloud1",
			CredentialID: "cred1",
		},
		// model2 appears once, using another credential (cred2)
		{
			SecretBackendRow: SecretBackendRow{
				ID:          "uuid-k8s",
				Name:        "kubernetes",
				BackendType: "kubernetes",
			},
			ModelName:    "model2",
			CloudID:      "cloud1",
			CredentialID: "cred2",
		},
		// a second model1 appears once, using credential cred2,
		// but with another UUID, simulating a model with another qualifier
		{
			SecretBackendRow: SecretBackendRow{
				ID:          "uuid-k8s",
				Name:        "kubernetes",
				BackendType: "kubernetes",
			},
			ModelUUID:    "model1-uuid-bis",
			ModelName:    "model1",
			CloudID:      "cloud1",
			CredentialID: "cred2",
		},
	}
	cldData := cloudRows{
		{
			ID:                "cloud1",
			Name:              "cloud1-name",
			Endpoint:          "https://cloud1.com",
			SkipTLSVerify:     true,
			IsControllerCloud: false,
			CACert:            "ca-cert1",
		},
	}
	credData := cloudCredentialRows{
		{
			ID:             "cred1",
			Name:           "cred1-name",
			AuthType:       "userpass",
			AttributeKey:   k8scloud.CredAttrUsername,
			AttributeValue: "my-user",
		},
		{
			ID:             "cred1",
			Name:           "cred1-name",
			AuthType:       "userpass",
			AttributeKey:   k8scloud.CredAttrPassword,
			AttributeValue: "my-password",
		},
		{
			ID:             "cred2",
			Name:           "cred2-name",
			AuthType:       "token",
			AttributeKey:   k8scloud.CredAttrToken,
			AttributeValue: "my-token",
		},
	}

	result, err := rows.toSecretBackend("my-controller", cldData, credData)
	c.Assert(err, tc.IsNil)

	// We still expect exactly one backend per model, even though the
	// underlying query returned multiple rows for model1.
	c.Assert(result, tc.HasLen, 3)

	c.Assert(result[0].Name, tc.Equals, "model1-local")
	c.Assert(result[0].Config["namespace"], tc.Equals, "model1")
	c.Assert(result[0].Config["username"], tc.Equals, "my-user")
	c.Assert(result[0].Config["password"], tc.Equals, "my-password")
	c.Assert(result[1].Name, tc.Equals, "model2-local")
	c.Assert(result[1].Config["namespace"], tc.Equals, "model2")
	c.Assert(result[1].Config["token"], tc.Equals, "my-token")
	c.Assert(result[2].Name, tc.Equals, "model1-local")
	c.Assert(result[2].Config["namespace"], tc.Equals, "model1")
	c.Assert(result[2].Config["token"], tc.Equals, "my-token")
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
