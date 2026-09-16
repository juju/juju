// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"context"
	"database/sql"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
	"github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

// networkIntegrationSuite exercises network state through the model schema.
// Controller peer-relation fixtures belong here because the FQDN decision spans
// application, relation, and network persistence.
type networkIntegrationSuite struct {
	testing.ModelSuite

	state   *state.State
	service *service.ProviderService
}

func TestNetworkIntegrationSuite(t *stdtesting.T) {
	tc.Run(t, &networkIntegrationSuite{})
}

func (s *networkIntegrationSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = state.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	s.service = service.NewProviderService(
		s.state,
		func(context.Context) (service.ProviderWithNetworking, error) {
			return nil, coreerrors.NotSupported
		},
		nil,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *networkIntegrationSuite) TestGetUnitEndpointNetworksRejectsUnknownUnit(c *tc.C) {
	infos, err := s.service.GetUnitEndpointNetworks(
		c.Context(), coreunit.Name("controller/0"), []string{"api"},
	)

	c.Check(infos, tc.IsNil)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *networkIntegrationSuite) TestGetUnitEndpointNetworksDoesNotExposePodFQDN(c *tc.C) {
	unitName := s.insertUnit(c, "controller", coreunit.Name("controller/0"), true, true)

	infos, err := s.service.GetUnitEndpointNetworks(
		c.Context(), unitName, []string{"api", "dbcluster"},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 2)
	for _, info := range infos {
		c.Check(info.IngressAddresses, tc.HasLen, 0)
		c.Check(info.DeviceInfos, tc.HasLen, 0)
	}
}

func (s *networkIntegrationSuite) TestGetUnitEndpointNetworksIaasControllerDoesNotExposePodFQDN(c *tc.C) {
	unitName := s.insertUnit(c, "controller-iaas", coreunit.Name("controller-iaas/0"), false, false)

	infos, err := s.service.GetUnitEndpointNetworks(c.Context(), unitName, []string{"api"})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 1)
	c.Check(infos[0].IngressAddresses, tc.HasLen, 0)
	c.Check(infos[0].DeviceInfos, tc.HasLen, 0)
}

func (s *networkIntegrationSuite) TestGetUnitEndpointNetworksIaasApplicationDoesNotExposePodFQDN(c *tc.C) {
	unitName := s.insertUnit(c, "mysql", coreunit.Name("mysql/0"), false, false)

	infos, err := s.service.GetUnitEndpointNetworks(c.Context(), unitName, []string{"api"})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 1)
	c.Check(infos[0].IngressAddresses, tc.HasLen, 0)
	c.Check(infos[0].DeviceInfos, tc.HasLen, 0)
}

func (s *networkIntegrationSuite) TestGetUnitEndpointNetworksCaasApplicationDoesNotExposePodFQDN(c *tc.C) {
	unitName := s.insertUnit(c, "redis", coreunit.Name("redis/0"), true, false)

	infos, err := s.service.GetUnitEndpointNetworks(c.Context(), unitName, []string{"api"})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 1)
	c.Check(infos[0].IngressAddresses, tc.HasLen, 0)
	c.Check(infos[0].DeviceInfos, tc.HasLen, 0)
}

func (s *networkIntegrationSuite) insertUnit(
	c *tc.C, applicationName string, unitName coreunit.Name, isCaas, addFQDN bool,
) coreunit.Name {
	charmUUID := uuid.MustNewUUID().String()
	appUUID := uuid.MustNewUUID().String()
	nodeUUID := uuid.MustNewUUID().String()
	unitUUID := uuid.MustNewUUID().String()
	fqdnUUID := uuid.MustNewUUID().String()
	fqdn := "controller-0.controller-service-endpoints.test.svc.cluster.local"

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		statements := []struct {
			query string
			args  []any
		}{
			{query: `INSERT INTO charm (uuid, reference_name, create_time) VALUES (?, ?, ?)`, args: []any{charmUUID, applicationName, time.Now()}},
			{
				query: `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, 0, ?, ?)`,
				args:  []any{appUUID, applicationName, charmUUID, corenetwork.AlphaSpaceId.String()},
			},
			{query: `INSERT INTO net_node (uuid) VALUES (?)`, args: []any{nodeUUID}},
			{
				query: `INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, 0, ?, ?, ?)`,
				args:  []any{unitUUID, unitName.String(), appUUID, charmUUID, nodeUUID},
			},
		}
		if isCaas {
			statements = append(statements, struct {
				query string
				args  []any
			}{`INSERT INTO k8s_pod (unit_uuid, provider_id) VALUES (?, ?)`, []any{unitUUID, unitName.String()}})
		}
		if addFQDN {
			statements = append(statements,
				struct {
					query string
					args  []any
				}{query: `INSERT INTO fqdn_address (uuid, address, scope_id) VALUES (?, ?, 1)`, args: []any{fqdnUUID, fqdn}},
				struct {
					query string
					args  []any
				}{query: `INSERT INTO net_node_fqdn_address (net_node_uuid, address_uuid) VALUES (?, ?)`, args: []any{nodeUUID, fqdnUUID}},
			)
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return unitName
}
