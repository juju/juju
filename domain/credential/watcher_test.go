// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credential_test

import (
	"context"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	userstate "github.com/juju/juju/domain/access/state"
	dbcloud "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/credential/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
	userUUID       user.UUID
	userName       user.Name
	controllerUUID string
}

func TestWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.controllerUUID = s.SeedControllerUUID(c)

	s.userName = usertesting.GenNewName(c, "test-user")
	s.userUUID = s.addOwner(c, s.userName)

	s.addCloud(c, s.userName, cloud.Cloud{
		Name:      "stratus",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
}

func (s *watcherSuite) TestWatchCloud(c *tc.C) {
	logger := loggertesting.WrapCheckLog(c)
	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "cloud")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, logger)
	st := state.NewState(func() (database.TxnRunner, error) { return watchableDBFactory() })

	service := service.NewWatchableService(st, watcherFactory, loggertesting.WrapCheckLog(c))

	key := corecredential.Key{
		Cloud: "stratus",
		Owner: s.userName,
		Name:  "foobar",
	}
	s.createCloudCredential(c, st, key)

	watcher, err := service.WatchCredential(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(func(c *tc.C) {
		credInfo := credential.CloudCredentialInfo{
			AuthType: string(cloud.AccessKeyAuthType),
			Attributes: map[string]string{
				"foo": "foo val",
				"bar": "bar val",
			},
			Revoked: true,
			Label:   "foobar",
		}
		err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			return st.UpsertCloudCredential(ctx, key, credInfo)
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		// Get the change.
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) addCloud(c *tc.C, userName user.Name, cloud cloud.Cloud) string {
	cloudSt := dbcloud.NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	cloudUUID := uuid.MustNewUUID().String()
	err := cloudSt.CreateCloud(ctx, userName, cloudUUID, cloud)
	c.Assert(err, tc.ErrorIsNil)

	return cloudUUID
}

func (s *watcherSuite) addOwner(c *tc.C, name user.Name) user.UUID {
	userUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	userState := userstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = userState.AddUserWithPermission(
		c.Context(),
		userUUID,
		name,
		"test user",
		false,
		userUUID,
		permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.controllerUUID,
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	return userUUID
}

func (s *watcherSuite) createCloudCredential(c *tc.C, st *state.State, key corecredential.Key) credential.CloudCredentialInfo {
	authType := cloud.AccessKeyAuthType
	attributes := map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	}

	credInfo := credential.CloudCredentialInfo{
		Label:      key.Name,
		AuthType:   string(authType),
		Attributes: attributes,
	}
	err := st.UpsertCloudCredential(c.Context(), key, credInfo)
	c.Assert(err, tc.ErrorIsNil)
	return credInfo
}
