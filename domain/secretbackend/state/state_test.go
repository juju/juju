// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite

	state *State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))
}

func (s *stateSuite) assertSecretBackend(
	c *gc.C, expectedSecretBackend coresecrets.SecretBackend, expectedNextRotationTime *time.Time,
) {
	db := s.DB()
	row := db.QueryRow(`
SELECT uuid, name, backend_type, token_rotate_interval
FROM secret_backend
WHERE uuid = ?`[1:], expectedSecretBackend.ID)
	c.Assert(row.Err(), gc.IsNil)

	var (
		actual              coresecrets.SecretBackend
		tokenRotateInterval NullableDuration
	)
	err := row.Scan(&actual.ID, &actual.Name, &actual.BackendType, &tokenRotateInterval)
	c.Assert(err, gc.IsNil)

	if tokenRotateInterval.Valid {
		actual.TokenRotateInterval = &tokenRotateInterval.Duration
	}
	if expectedNextRotationTime != nil {
		var actualNextRotationTime sql.NullTime
		row = db.QueryRow(`
SELECT next_rotation_time
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], expectedSecretBackend.ID)
		c.Assert(row.Err(), gc.IsNil)
		err = row.Scan(&actualNextRotationTime)
		c.Assert(err, gc.IsNil)
		c.Assert(actualNextRotationTime.Valid, jc.IsTrue)
		c.Assert(actualNextRotationTime.Time.Equal(*expectedNextRotationTime), jc.IsTrue)
	} else {
		row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], expectedSecretBackend.ID)
		var count int
		err = row.Scan(&count)
		c.Assert(err, gc.IsNil)
		c.Assert(count, gc.Equals, 0)
	}

	if len(expectedSecretBackend.Config) > 0 {
		actual.Config = map[string]interface{}{}
		rows, err := db.Query(`
SELECT name, content
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], expectedSecretBackend.ID)
		c.Assert(err, gc.IsNil)
		c.Assert(rows.Err(), gc.IsNil)
		defer rows.Close()
		for rows.Next() {
			var k, v string
			err = rows.Scan(&k, &v)
			c.Assert(err, gc.IsNil)
			actual.Config[k] = v
		}
	} else {
		var count int
		row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], expectedSecretBackend.ID)
		err = row.Scan(&count)
		c.Assert(err, gc.IsNil)
		c.Assert(count, gc.Equals, 0)
	}
	c.Assert(actual, gc.DeepEquals, expectedSecretBackend)
}

func (s *stateSuite) TestCreateSecretBackend(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	result, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, backendID)

	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)

	_, err = s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, jc.ErrorIs, domain.ErrDuplicate)
}

func (s *stateSuite) TestCreateSecretBackendWithNoRotateNoConfig(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	result, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, backendID)

	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	}, nil)
}

func (s *stateSuite) TestUpdateSecretBackendInvalidArg(c *gc.C) {
	err := s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{})
	c.Assert(err, gc.ErrorMatches, `backend ID is missing`)
}

func (s *stateSuite) TestUpdateSecretBackend(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)

	newRotateInternal := 48 * time.Hour
	newNextRotateTime := time.Now().Add(newRotateInternal)
	nameChange := "my-backend-updated"
	err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		ID:                  backendID,
		NameChange:          &nameChange,
		TokenRotateInterval: &newRotateInternal,
		NextRotateTime:      &newNextRotateTime,
		Config: map[string]interface{}{
			"key1": "value1-updated",
			"key3": "value3",
		},
	})
	c.Assert(err, gc.IsNil)

	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend-updated",
		BackendType:         "vault",
		TokenRotateInterval: &newRotateInternal,
		Config: map[string]interface{}{
			"key1": "value1-updated",
			"key2": "value2",
			"key3": "value3",
		},
	}, &newNextRotateTime)
}

func (s *stateSuite) TestUpdateSecretBackendWithNoRotateNoConfig(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)

	nameChange := "my-backend-updated"
	err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		ID:         backendID,
		NameChange: &nameChange,
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend-updated",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)
}

func (s *stateSuite) TestUpdateSecretBackendNameAlreadyExists(c *gc.C) {
	backendID1 := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:                  backendID1,
		Name:                "my-backend1",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
	})
	c.Assert(err, gc.IsNil)

	backendID2 := uuid.MustNewUUID().String()
	_, err = s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:                  backendID2,
		Name:                "my-backend2",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
	})
	c.Assert(err, gc.IsNil)

	nameChange := "my-backend1"
	err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		ID:         backendID2,
		NameChange: &nameChange,
	})
	c.Assert(err, jc.ErrorIs, domain.ErrDuplicate)
}

func (s *stateSuite) TestDeleteSecretBackend(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)

	err = s.state.DeleteSecretBackend(context.Background(), backendID, false)
	c.Assert(err, gc.IsNil)

	db := s.DB()
	row := db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend
WHERE uuid = ?`[1:], backendID)
	var count int
	err = row.Scan(&count)
	c.Assert(err, gc.IsNil)
	c.Assert(count, gc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], backendID)
	err = row.Scan(&count)
	c.Assert(err, gc.IsNil)
	c.Assert(count, gc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], backendID)
	err = row.Scan(&count)
	c.Assert(err, gc.IsNil)
	c.Assert(count, gc.Equals, 0)
}

func (s *stateSuite) TestDeleteSecretBackendWithNoConfigNoNextRotationTime(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	}, nil)

	err = s.state.DeleteSecretBackend(context.Background(), backendID, false)
	c.Assert(err, gc.IsNil)

	db := s.DB()
	row := db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend
WHERE uuid = ?`[1:], backendID)
	var count int
	err = row.Scan(&count)
	c.Assert(err, gc.IsNil)
	c.Assert(count, gc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], backendID)
	err = row.Scan(&count)
	c.Assert(err, gc.IsNil)
	c.Assert(count, gc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], backendID)
	err = row.Scan(&count)
	c.Assert(err, gc.IsNil)
	c.Assert(count, gc.Equals, 0)
}

func (s *stateSuite) TestDeleteSecretBackendInUseFail(c *gc.C) {
	c.Skip("TODO: wait for secret DqLite support")
}

func (s *stateSuite) TestDeleteSecretBackendInUseWithForce(c *gc.C) {
	c.Skip("TODO: wait for secret DqLite support")
}

func (s *stateSuite) TestListSecretBackends(c *gc.C) {
	backendID1 := uuid.MustNewUUID().String()
	rotateInternal1 := 24 * time.Hour
	nextRotateTime1 := time.Now().Add(rotateInternal1)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:                  backendID1,
		Name:                "my-backend1",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		NextRotateTime:      &nextRotateTime1,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:                  backendID1,
		Name:                "my-backend1",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime1)

	backendID2 := uuid.MustNewUUID().String()
	rotateInternal2 := 48 * time.Hour
	nextRotateTime2 := time.Now().Add(rotateInternal2)
	_, err = s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:                  backendID2,
		Name:                "my-backend2",
		BackendType:         "kubernetes",
		TokenRotateInterval: &rotateInternal2,
		NextRotateTime:      &nextRotateTime2,
		Config: map[string]interface{}{
			"key3": "value3",
			"key4": "value4",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:                  backendID2,
		Name:                "my-backend2",
		BackendType:         "kubernetes",
		TokenRotateInterval: &rotateInternal2,
		Config: map[string]interface{}{
			"key3": "value3",
			"key4": "value4",
		},
	}, &nextRotateTime2)

	backends, err := s.state.ListSecretBackends(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(backends, gc.HasLen, 2)
	sort.Slice(backends, func(i, j int) bool {
		return backends[i].Name < backends[j].Name
	})
	c.Assert(backends, gc.DeepEquals, []*secretbackend.SecretBackendInfo{
		{
			SecretBackend: coresecrets.SecretBackend{
				ID:                  backendID1,
				Name:                "my-backend1",
				BackendType:         "vault",
				TokenRotateInterval: &rotateInternal1,
				Config: map[string]interface{}{
					"key1": "value1",
					"key2": "value2",
				},
			},
		},
		{
			SecretBackend: coresecrets.SecretBackend{
				ID:                  backendID2,
				Name:                "my-backend2",
				BackendType:         "kubernetes",
				TokenRotateInterval: &rotateInternal2,
				Config: map[string]interface{}{
					"key3": "value3",
					"key4": "value4",
				},
			},
		},
	})
}

func (s *stateSuite) TestListSecretBackendsEmpty(c *gc.C) {
	backends, err := s.state.ListSecretBackends(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(backends, gc.IsNil)
}

func (s *stateSuite) TestGetSecretBackendByName(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	}, nil)

	backend, err := s.state.GetSecretBackendByName(context.Background(), "my-backend")
	c.Assert(err, gc.IsNil)
	c.Assert(backend, gc.DeepEquals, &coresecrets.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	})

	err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		ID:                  backendID,
		TokenRotateInterval: &rotateInternal,
	})
	c.Assert(err, gc.IsNil)
	backend, err = s.state.GetSecretBackendByName(context.Background(), "my-backend")
	c.Assert(err, gc.IsNil)
	c.Assert(backend, gc.DeepEquals, &coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	})

	err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		ID: backendID,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	backend, err = s.state.GetSecretBackendByName(context.Background(), "my-backend")
	c.Assert(err, gc.IsNil)
	c.Assert(backend, gc.DeepEquals, &coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
}

func (s *stateSuite) TestGetSecretBackendByNameNotFound(c *gc.C) {
	backend, err := s.state.GetSecretBackendByName(context.Background(), "my-backend")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(err, gc.ErrorMatches, `secret backend "my-backend" not found`)
	c.Assert(backend, gc.IsNil)
}

func (s *stateSuite) TestGetSecretBackend(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	}, nil)

	backend, err := s.state.GetSecretBackend(context.Background(), backendID)
	c.Assert(err, gc.IsNil)
	c.Assert(backend, gc.DeepEquals, &coresecrets.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	})

	err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		ID:                  backendID,
		TokenRotateInterval: &rotateInternal,
	})
	c.Assert(err, gc.IsNil)
	backend, err = s.state.GetSecretBackend(context.Background(), backendID)
	c.Assert(err, gc.IsNil)
	c.Assert(backend, gc.DeepEquals, &coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	})

	err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		ID: backendID,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	backend, err = s.state.GetSecretBackend(context.Background(), backendID)
	c.Assert(err, gc.IsNil)
	c.Assert(backend, gc.DeepEquals, &coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
}

func (s *stateSuite) TestGetSecretBackendNotFound(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	backend, err := s.state.GetSecretBackend(context.Background(), backendID)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(err, gc.ErrorMatches, `secret backend "`+backendID+`" not found`)
	c.Assert(backend, gc.IsNil)
}

func (s *stateSuite) TestSecretBackendRotated(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	}, &nextRotateTime)

	newNextRotateTime := time.Now().Add(2 * rotateInternal)
	err = s.state.SecretBackendRotated(context.Background(), backendID, newNextRotateTime)
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	},
		// No ops because the new next rotation time is after the current one.
		&nextRotateTime,
	)

	newNextRotateTime = time.Now().Add(rotateInternal / 2)
	err = s.state.SecretBackendRotated(context.Background(), backendID, newNextRotateTime)
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, coresecrets.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	},
		// The next rotation time is updated.
		&newNextRotateTime,
	)

	nonExistBackendID := uuid.MustNewUUID().String()
	newNextRotateTime = time.Now().Add(rotateInternal / 4)
	err = s.state.SecretBackendRotated(context.Background(), nonExistBackendID, newNextRotateTime)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(err, gc.ErrorMatches, `secret backend "`+nonExistBackendID+`" not found`)
}

func (s *stateSuite) TestWatchSecretBackendRotationChanges(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	backendID1 := uuid.MustNewUUID().String()
	backendID2 := uuid.MustNewUUID().String()
	nextRotateTime1 := time.Now().Add(12 * time.Hour)
	nextRotateTime2 := time.Now().Add(24 * time.Hour)

	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:             backendID1,
		Name:           "my-backend1",
		BackendType:    "vault",
		NextRotateTime: &nextRotateTime1,
	})
	c.Assert(err, gc.IsNil)

	_, err = s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		ID:             backendID2,
		Name:           "my-backend2",
		BackendType:    "kubernetes",
		NextRotateTime: &nextRotateTime2,
	})
	c.Assert(err, gc.IsNil)

	ch := make(chan []string)
	mockWatcher := NewMockStringsWatcher(ctrl)
	mockWatcher.EXPECT().Changes().Return(ch).AnyTimes()

	watcherFactory := NewMockWatcherFactory(ctrl)
	watcherFactory.EXPECT().NewNamespaceWatcher(
		"secret_backend_rotation", changestream.All, `SELECT backend_uuid FROM secret_backend_rotation`,
	).Return(mockWatcher, nil)

	w, err := s.state.WatchSecretBackendRotationChanges(watcherFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, w) })
	select {
	case <-w.Changes():
		// consume the initial empty change then send the backend IDs
		go func() {
			ch <- []string{backendID1, backendID2}
		}()
	case <-time.After(jujutesting.ShortWait):
	}

	select {
	case changes, ok := <-w.Changes():
		c.Assert(ok, gc.Equals, true)
		c.Assert(changes, gc.HasLen, 2)
		sort.Slice(changes, func(i, j int) bool {
			return changes[i].Name < changes[j].Name
		})

		c.Assert(changes[0].ID, gc.Equals, backendID1)
		c.Assert(changes[0].Name, gc.Equals, "my-backend1")
		c.Assert(changes[0].NextTriggerTime.Equal(nextRotateTime1), jc.IsTrue)
		c.Assert(changes[1].ID, gc.Equals, backendID2)
		c.Assert(changes[1].Name, gc.Equals, "my-backend2")
		c.Assert(changes[1].NextTriggerTime.Equal(nextRotateTime2), jc.IsTrue)
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for backend rotation changes")
	}
}
