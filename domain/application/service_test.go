// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/life"
	coresecrets "github.com/juju/juju/core/secrets"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/schema/testing"
	domainsecret "github.com/juju/juju/domain/secret"
	secretstate "github.com/juju/juju/domain/secret/state"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testing.ModelSuite

	svc         *service.Service
	secretState *secretstate.State
}

var _ = gc.Suite(&serviceSuite{})

func ptr[T any](v T) *T {
	return &v
}

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.secretState = secretstate.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, loggertesting.WrapCheckLog(c))
	s.svc = service.NewService(
		state.NewApplicationState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil },
			loggertesting.WrapCheckLog(c),
		),
		secretstate.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil },
			loggertesting.WrapCheckLog(c),
		),
		state.NewCharmState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }),
		state.NewResourceState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil },
			loggertesting.WrapCheckLog(c),
		),
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return provider.CommonStorageProviders()
		}),
		nil,
		loggertesting.WrapCheckLog(c),
	)

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, target_agent_version, name, type, cloud, cloud_type)
			VALUES (?, ?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id(), jujuversion.Current.String())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) createApplication(c *gc.C, name string, units ...service.AddUnitArg) coreapplication.ID {
	appID, err := s.svc.CreateApplication(context.Background(), name, &stubCharm{}, corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, service.AddApplicationArgs{
		ReferenceName: name,
	}, units...)
	c.Assert(err, jc.ErrorIsNil)
	return appID
}

func (s *serviceSuite) TestGetApplicationLife(c *gc.C) {
	s.createApplication(c, "foo")

	lifeValue, err := s.svc.GetApplicationLife(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lifeValue, gc.Equals, life.Alive)

	_, err = s.svc.GetApplicationLife(context.Background(), "bar")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *serviceSuite) TestDestroyApplication(c *gc.C) {
	appID := s.createApplication(c, "foo")

	err := s.svc.DestroyApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	var gotLife int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT life_id FROM application WHERE uuid = ?", appID).
			Scan(&gotLife)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotLife, gc.Equals, 1)
}

func (s *serviceSuite) createSecrets(c *gc.C, appUUID coreapplication.ID, unitName string) (appSecretURI *coresecrets.URI, unitSecretURI *coresecrets.URI) {
	ctx := context.Background()
	appSecretURI = coresecrets.NewURI()
	sp := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	_ = s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		err := s.secretState.CreateCharmApplicationSecret(ctx, 1, appSecretURI, appUUID, sp)
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	if unitName == "" {
		return appSecretURI, unitSecretURI
	}

	unitSecretURI = coresecrets.NewURI()
	sp2 := domainsecret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo": "bar"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	_ = s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		unitUUID, err := s.secretState.GetUnitUUID(ctx, unitName)
		c.Assert(err, jc.ErrorIsNil)
		err = s.secretState.CreateCharmUnitSecret(ctx, 1, unitSecretURI, unitUUID, sp2)
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	return appSecretURI, unitSecretURI
}

func (s *serviceSuite) TestDeleteApplication(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appUUID := s.createApplication(c, "foo")
	s.createSecrets(c, appUUID, "")

	err := s.svc.DeleteApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	var (
		gotAppCount    int
		gotSecretCount int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE name = ?", "foo").
			Scan(&gotAppCount)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_metadata sm JOIN secret_application_owner so ON sm.secret_id = so.secret_id WHERE application_uuid = ?", appUUID).
			Scan(&gotSecretCount)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotAppCount, gc.Equals, 0)
	c.Assert(gotSecretCount, gc.Equals, 0)
}

func (s *serviceSuite) TestDeleteApplicationNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	err := s.svc.DeleteApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *serviceSuite) TestEnsureApplicationDead(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo")

	err := s.svc.EnsureApplicationDead(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	var gotLife int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT life_id FROM application WHERE name = ?", "foo").
			Scan(&gotLife)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotLife, gc.Equals, 2)
}

func (s *serviceSuite) TestEnsureApplicationDeadNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	err := s.svc.EnsureApplicationDead(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetUnitLife(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", u)

	lifeValue, err := s.svc.GetUnitLife(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lifeValue, gc.Equals, life.Alive)

	_, err = s.svc.GetUnitLife(context.Background(), "foo/667")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) TestDestroyUnit(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", u)

	err := s.svc.DestroyUnit(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)

	var gotLife int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT life_id FROM unit WHERE name = ?", u.UnitName).
			Scan(&gotLife)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotLife, gc.Equals, 1)
}

func (s *serviceSuite) TestEnsureUnitDead(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unitName := coreunit.Name("foo/666")
	u := service.AddUnitArg{
		UnitName: unitName,
	}
	s.createApplication(c, "foo", u)

	revoker := application.NewMockRevoker(ctrl)
	revoker.EXPECT().RevokeLeadership("foo", unitName)

	err := s.svc.EnsureUnitDead(context.Background(), unitName, revoker)
	c.Assert(err, jc.ErrorIsNil)

	var gotLife int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT life_id FROM unit WHERE name = ?", u.UnitName).
			Scan(&gotLife)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotLife, gc.Equals, 2)
}

func (s *serviceSuite) TestEnsureUnitDeadNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo")

	revoker := application.NewMockRevoker(ctrl)

	err := s.svc.EnsureUnitDead(context.Background(), coreunit.Name("foo/666"), revoker)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	u := service.AddUnitArg{
		UnitName: "foo/666",
	}
	appUUID := s.createApplication(c, "foo", u)
	s.createSecrets(c, appUUID, "foo/666")

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name = ?", u.UnitName)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.svc.DeleteUnit(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)

	var (
		gotUnitCount   int
		gotSecretCount int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name = ?", u.UnitName).
			Scan(&gotUnitCount)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_metadata sm JOIN secret_unit_owner so ON sm.secret_id = so.secret_id JOIN unit u ON so.unit_uuid = u.uuid WHERE u.name = ?", u.UnitName).
			Scan(&gotSecretCount)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotUnitCount, gc.Equals, 0)
	c.Assert(gotSecretCount, gc.Equals, 0)
}

func (s *serviceSuite) TestDeleteUnitNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo")

	err := s.svc.DeleteUnit(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unitName := coreunit.Name("foo/666")
	u := service.AddUnitArg{
		UnitName: unitName,
	}
	s.createApplication(c, "foo", u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name = ?", u.UnitName)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	revoker := application.NewMockRevoker(ctrl)
	revoker.EXPECT().RevokeLeadership("foo", unitName)

	err = s.svc.RemoveUnit(context.Background(), unitName, revoker)
	c.Assert(err, jc.ErrorIsNil)

	var gotCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name = ?", u.UnitName).
			Scan(&gotCount)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotCount, gc.Equals, 0)
}

func (s *serviceSuite) TestRemoveUnitStillAlive(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	u := service.AddUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", u)

	revoker := application.NewMockRevoker(ctrl)

	err := s.svc.RemoveUnit(context.Background(), "foo/666", revoker)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitIsAlive)
}

func (s *serviceSuite) TestRemoveUnitNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.createApplication(c, "foo")

	revoker := application.NewMockRevoker(ctrl)

	err := s.svc.RemoveUnit(context.Background(), "foo/666", revoker)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) assertCAASUnit(c *gc.C, name, passwordHash, addressValue string, ports []string) {
	var (
		gotPasswordHash  string
		gotAddress       string
		gotAddressType   ipaddress.AddressType
		gotAddressScope  ipaddress.Scope
		gotAddressOrigin ipaddress.Origin
		gotPorts         []string
	)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT password_hash FROM unit WHERE name = ?", name).Scan(&gotPasswordHash)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `
SELECT address_value, type_id, scope_id, origin_id FROM ip_address ipa
JOIN link_layer_device lld ON lld.uuid = ipa.device_uuid
JOIN unit u ON u.net_node_uuid = lld.net_node_uuid WHERE u.name = ?
`, name).
			Scan(&gotAddress, &gotAddressType, &gotAddressScope, &gotAddressOrigin)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `
SELECT port FROM cloud_container_port ccp
JOIN cloud_container cc ON cc.net_node_uuid = ccp.cloud_container_uuid
JOIN unit u ON u.net_node_uuid = cc.net_node_uuid WHERE u.name = ?
`, name)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var port string
			err = rows.Scan(&port)
			if err != nil {
				return err
			}
			gotPorts = append(gotPorts, port)
		}
		return rows.Err()
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotPasswordHash, gc.Equals, passwordHash)
	c.Assert(gotAddress, gc.Equals, addressValue)
	c.Assert(gotAddressType, gc.Equals, ipaddress.AddressTypeIPv4)
	c.Assert(gotAddressScope, gc.Equals, ipaddress.ScopeMachineLocal)
	c.Assert(gotAddressOrigin, gc.Equals, ipaddress.OriginProvider)
	c.Assert(gotPorts, jc.DeepEquals, ports)
}

func (s *serviceSuite) TestReplaceCAASUnit(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/1",
	}
	s.createApplication(c, "foo", u)

	address := "10.6.6.6"
	ports := []string{"666"}
	args := service.RegisterCAASUnitParams{
		UnitName:     "foo/1",
		PasswordHash: "passwordhash",
		ProviderId:   "provider-id",
		Address:      ptr(address),
		Ports:        ptr(ports),
		OrderedScale: true,
		OrderedId:    1,
	}
	err := s.svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCAASUnit(c, "foo/1", "passwordhash", address, ports)
}

func (s *serviceSuite) TestReplaceDeadCAASUnit(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/1",
	}
	s.createApplication(c, "foo", u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name = ?", u.UnitName)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	args := service.RegisterCAASUnitParams{
		UnitName:     "foo/1",
		PasswordHash: "passwordhash",
		ProviderId:   "provider-id",
		OrderedScale: true,
		OrderedId:    1,
	}
	err = s.svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitAlreadyExists)
}

func (s *serviceSuite) TestNewCAASUnit(c *gc.C) {
	appID := s.createApplication(c, "foo")

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_scale SET scale = 2 WHERE application_uuid = ?", appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	address := "10.6.6.6"
	ports := []string{"666"}
	args := service.RegisterCAASUnitParams{
		UnitName:     "foo/1",
		PasswordHash: "passwordhash",
		ProviderId:   "provider-id",
		Address:      &address,
		Ports:        &ports,
		OrderedScale: true,
		OrderedId:    1,
	}
	err = s.svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCAASUnit(c, "foo/1", "passwordhash", address, ports)
}

func (s *serviceSuite) TestRegisterCAASUnitExceedsScale(c *gc.C) {
	s.createApplication(c, "foo")

	args := service.RegisterCAASUnitParams{
		UnitName:     "foo/1",
		PasswordHash: "passwordhash",
		ProviderId:   "provider-id",
		OrderedScale: true,
		OrderedId:    666,
	}
	err := s.svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotAssigned)
}

func (s *serviceSuite) TestRegisterCAASUnitExceedsScaleTarget(c *gc.C) {
	appID := s.createApplication(c, "foo")

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_scale SET scale = 3, scale_target = 1, scaling = true WHERE application_uuid = ?", appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	args := service.RegisterCAASUnitParams{
		UnitName:     "foo/1",
		PasswordHash: "passwordhash",
		ProviderId:   "provider-id",
		OrderedScale: true,
		OrderedId:    2,
	}
	err = s.svc.RegisterCAASUnit(context.Background(), "foo", args)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotAssigned)
}

func (s *serviceSuite) TestSetScalingState(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/1",
	}
	appID := s.createApplication(c, "foo", u)

	err := s.svc.SetApplicationScalingState(context.Background(), "foo", 1, true)
	c.Assert(err, jc.ErrorIsNil)

	var (
		gotScaleTarget int
		gotScaling     bool
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale_target, scaling FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScaleTarget, &gotScaling)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScaleTarget, gc.Equals, 1)
	c.Assert(gotScaling, jc.IsTrue)
}

func (s *serviceSuite) TestSetScalingStateAlreadyScaling(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/1",
	}
	appID := s.createApplication(c, "foo", u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_scale SET scaling = true WHERE application_uuid = ?", appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.svc.SetApplicationScalingState(context.Background(), "foo", 666, true)
	c.Assert(err, jc.ErrorIsNil)

	var (
		gotScaleTarget int
		gotScaling     bool
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale_target, scaling FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScaleTarget, &gotScaling)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScaleTarget, gc.Equals, 666)
	c.Assert(gotScaling, jc.IsTrue)
}

func (s *serviceSuite) TestSetScalingStateDying(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/1",
	}
	appID := s.createApplication(c, "foo", u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = 1 WHERE uuid = ?", appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.svc.SetApplicationScalingState(context.Background(), "foo", 666, true)
	c.Assert(err, jc.ErrorIsNil)

	var (
		gotScaleTarget int
		gotScaling     bool
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale_target, scaling FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScaleTarget, &gotScaling)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScaleTarget, gc.Equals, 666)
	c.Assert(gotScaling, jc.IsTrue)
}

func (s *serviceSuite) TestSetScalingStateInconsistent(c *gc.C) {
	s.createApplication(c, "foo")

	err := s.svc.SetApplicationScalingState(context.Background(), "foo", 666, true)
	c.Assert(err, jc.ErrorIs, applicationerrors.ScalingStateInconsistent)
}

func (s *serviceSuite) TestGetScalingState(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/1",
	}
	appID := s.createApplication(c, "foo", u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application_scale SET scaling = true WHERE application_uuid = ?", appID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.svc.SetApplicationScalingState(context.Background(), "foo", 666, true)
	c.Assert(err, jc.ErrorIsNil)

	got, err := s.svc.GetApplicationScalingState(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, service.ScalingState{
		Scaling:     true,
		ScaleTarget: 666,
	})
}

func (s *serviceSuite) TestSetScale(c *gc.C) {
	appID := s.createApplication(c, "foo")

	err := s.svc.SetApplicationScale(context.Background(), "foo", 666)
	c.Assert(err, jc.ErrorIsNil)

	var gotScale int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale FROM application_scale WHERE application_uuid = ?", appID).
			Scan(&gotScale)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScale, gc.Equals, 666)
}

func (s *serviceSuite) TestGetScale(c *gc.C) {
	s.createApplication(c, "foo")

	err := s.svc.SetApplicationScale(context.Background(), "foo", 666)
	c.Assert(err, jc.ErrorIsNil)

	got, err := s.svc.GetApplicationScale(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, 666)
}

func (s *serviceSuite) TestChangeScale(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/1",
	}
	appID := s.createApplication(c, "foo", u)

	newScale, err := s.svc.ChangeApplicationScale(context.Background(), "foo", 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newScale, gc.Equals, 3)

	var gotScale int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT scale FROM application_scale WHERE application_uuid = ?", appID).Scan(&gotScale)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotScale, gc.Equals, 3)
}

func (s *serviceSuite) TestChangeScaleInvalid(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/1",
	}
	s.createApplication(c, "foo", u)

	_, err := s.svc.ChangeApplicationScale(context.Background(), "foo", -2)
	c.Assert(err, jc.ErrorIs, applicationerrors.ScaleChangeInvalid)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNumLessThanScale(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/0",
	}
	u2 := service.AddUnitArg{
		UnitName: "foo/1",
	}
	s.createApplication(c, "foo", u, u2)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := application.NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 6,
	}, nil)
	broker := application.NewMockBroker(ctrl)
	broker.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)
	willRestart, err := s.svc.CAASUnitTerminating(context.Background(), "foo", 1, broker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(willRestart, jc.IsTrue)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNumGreaterThanScale(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/0",
	}
	s.createApplication(c, "foo", u)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := application.NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 6,
	}, nil)
	broker := application.NewMockBroker(ctrl)
	broker.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)
	willRestart, err := s.svc.CAASUnitTerminating(context.Background(), "foo", 666, broker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(willRestart, jc.IsFalse)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNumLessThanDesired(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/0",
	}
	u2 := service.AddUnitArg{
		UnitName: "foo/1",
	}
	u3 := service.AddUnitArg{
		UnitName: "foo/2",
	}
	s.createApplication(c, "foo", u, u2, u3)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := application.NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 6,
	}, nil)
	broker := application.NewMockBroker(ctrl)
	broker.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)
	err := s.svc.SetApplicationScalingState(context.Background(), "foo", 6, false)
	c.Assert(err, jc.ErrorIsNil)

	willRestart, err := s.svc.CAASUnitTerminating(context.Background(), "foo", 2, broker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(willRestart, jc.IsTrue)
}

func (s *serviceSuite) TestCAASUnitTerminatingUnitNumGreaterThanDesired(c *gc.C) {
	u := service.AddUnitArg{
		UnitName: "foo/0",
	}
	u2 := service.AddUnitArg{
		UnitName: "foo/1",
	}
	u3 := service.AddUnitArg{
		UnitName: "foo/2",
	}
	s.createApplication(c, "foo", u, u2, u3)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := application.NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 1,
	}, nil)
	broker := application.NewMockBroker(ctrl)
	broker.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)
	err := s.svc.SetApplicationScalingState(context.Background(), "foo", 6, false)
	c.Assert(err, jc.ErrorIsNil)

	willRestart, err := s.svc.CAASUnitTerminating(context.Background(), "foo", 2, broker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(willRestart, jc.IsFalse)
}
