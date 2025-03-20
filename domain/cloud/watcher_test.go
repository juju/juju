// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"context"
	"database/sql"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/cloud/service"
	"github.com/juju/juju/domain/cloud/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
	adminUUID uuid.UUID
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.adminUUID = uuid.MustNewUUID()
	s.ensureUser(c, s.adminUUID.String(), "admin", s.adminUUID.String())
}

func (s *watcherSuite) TestWatchCloud(c *gc.C) {
	logger := loggertesting.WrapCheckLog(c)
	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "cloud")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, logger)
	st := state.NewState(func() (database.TxnRunner, error) { return watchableDBFactory() })

	service := service.NewWatchableService(st, watcherFactory)

	cloudUUID := uuid.MustNewUUID().String()

	cloud := testCloud

	err := st.CreateCloud(context.Background(), usertesting.GenNewName(c, "admin"), cloudUUID, cloud)
	c.Assert(err, jc.ErrorIsNil)

	watcher, err := service.WatchCloud(context.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(func(c *gc.C) {
		cloud.Endpoint = "https://endpoint2"
		err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			err := st.UpdateCloud(ctx, cloud)
			return err
		})

	}, func(w watchertest.WatcherC[struct{}]) {
		// Get the change.
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) ensureUser(c *gc.C, userUUID, name, createdByUUID string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, userUUID, name, name, false, false, createdByUUID, time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user_authentication (user_uuid, disabled)
			VALUES (?, ?)
		`, userUUID, false)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

var (
	testCloud = cloud.Cloud{
		Name:             "fluffy",
		Type:             "ec2",
		AuthTypes:        []cloud.AuthType{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Endpoint:         "https://endpoint",
		IdentityEndpoint: "https://identity-endpoint",
		StorageEndpoint:  "https://storage-endpoint",
		Regions: []cloud.Region{{
			Name:             "region1",
			Endpoint:         "http://region-endpoint1",
			IdentityEndpoint: "http://region-identity-endpoint1",
			StorageEndpoint:  "http://region-identity-endpoint1",
		}, {
			Name:             "region2",
			Endpoint:         "http://region-endpoint2",
			IdentityEndpoint: "http://region-identity-endpoint2",
			StorageEndpoint:  "http://region-identity-endpoint2",
		}},
		CACertificates:    []string{"cert1", "cert2"},
		SkipTLSVerify:     true,
		IsControllerCloud: false,
	}
)
