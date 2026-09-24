// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/life"
)

func (s *applicationStateSuite) TestK8sServiceAddressLifecycle(c *tc.C) {
	s.createCAASApplication(c, "foo", life.Alive)
	for _, test := range []struct {
		addresses network.ProviderAddresses
		want      []string
	}{
		{network.ProviderAddresses{
			network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
			network.NewMachineAddress("2001:db8::1", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			network.NewMachineAddress("lb.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		}, []string{"10.0.0.1|local-cloud", "2001:db8::1|public", "lb.example.com|public"}},
		{network.ProviderAddresses{
			network.NewMachineAddress("lb.example.com", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
		}, []string{"lb.example.com|local-cloud"}},
		{network.ProviderAddresses{
			network.NewMachineAddress("10.0.0.2", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
		}, []string{"10.0.0.2|local-cloud"}},
		{nil, nil},
		{network.ProviderAddresses{
			network.NewMachineAddress("lb.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		}, []string{"lb.example.com|public"}},
		{network.ProviderAddresses{}, nil},
	} {
		c.Assert(s.upsertK8sService(c, "foo", "provider", test.addresses), tc.ErrorIsNil)
		c.Check(s.k8sServiceAddresses(c, "foo"), tc.SameContents, test.want)
	}
	var count int
	c.Assert(s.DB().QueryRowContext(c.Context(), "SELECT COUNT(*) FROM fqdn_address").Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *applicationStateSuite) TestK8sServiceSharedHostname(c *tc.C) {
	s.createCAASApplication(c, "foo", life.Alive)
	s.createCAASApplication(c, "bar", life.Alive)
	addresses := network.ProviderAddresses{network.NewMachineAddress("shared.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress()}
	for _, app := range []string{"foo", "bar"} {
		c.Assert(s.upsertK8sService(c, app, app+"-provider", addresses), tc.ErrorIsNil)
	}
	// Recreating one Service with a different scope preserves the other
	// owner's source facts.
	c.Assert(s.upsertK8sService(c, "foo", "foo-recreated-provider", network.ProviderAddresses{
		network.NewMachineAddress("shared.example.com", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
	}), tc.ErrorIsNil)
	c.Check(s.k8sServiceAddresses(c, "foo"), tc.SameContents, []string{"shared.example.com|local-cloud"})
	c.Check(s.k8sServiceAddresses(c, "bar"), tc.SameContents, []string{"shared.example.com|public"})

	c.Assert(s.upsertK8sService(c, "foo", "foo-recreated-provider", nil), tc.ErrorIsNil)
	c.Check(s.k8sServiceAddresses(c, "bar"), tc.SameContents, []string{"shared.example.com|public"})
	// The surviving association still allows the removed owner to reattach.
	c.Assert(s.upsertK8sService(c, "foo", "foo-recreated-provider", addresses), tc.ErrorIsNil)
	c.Check(s.k8sServiceAddresses(c, "foo"), tc.SameContents, []string{"shared.example.com|public"})
}

func (s *applicationStateSuite) TestK8sServiceDuplicateHostname(c *tc.C) {
	s.createCAASApplication(c, "foo", life.Alive)
	address := network.NewMachineAddress("lb.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress()
	c.Assert(s.upsertK8sService(c, "foo", "provider", network.ProviderAddresses{
		address, address,
	}), tc.ErrorIsNil)
	c.Check(s.k8sServiceAddresses(c, "foo"), tc.SameContents, []string{"lb.example.com|public"})
	for table, want := range map[string]int{
		"fqdn_address":          1,
		"net_node_fqdn_address": 1,
		"link_layer_device":     0,
	} {
		var count int
		c.Assert(s.DB().QueryRowContext(c.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count), tc.ErrorIsNil)
		c.Check(count, tc.Equals, want, tc.Commentf("table %s", table))
	}
}

func (s *applicationStateSuite) TestK8sServiceRemoveMultipleHostnames(c *tc.C) {
	s.createCAASApplication(c, "foo", life.Alive)
	s.createCAASApplication(c, "bar", life.Alive)
	shared := network.NewMachineAddress("shared.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress()
	c.Assert(s.upsertK8sService(c, "foo", "foo-provider", network.ProviderAddresses{
		shared,
		network.NewMachineAddress("first.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		network.NewMachineAddress("second.example.com", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
	}), tc.ErrorIsNil)
	c.Assert(s.upsertK8sService(c, "bar", "bar-provider", network.ProviderAddresses{shared}), tc.ErrorIsNil)

	c.Assert(s.upsertK8sService(c, "foo", "foo-provider", nil), tc.ErrorIsNil)
	c.Check(s.k8sServiceAddresses(c, "foo"), tc.HasLen, 0)
	c.Check(s.k8sServiceAddresses(c, "bar"), tc.SameContents, []string{"shared.example.com|public"})
	var count int
	c.Assert(s.DB().QueryRowContext(c.Context(), "SELECT COUNT(*) FROM fqdn_address").Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

func (s *applicationStateSuite) TestK8sServiceSyntheticApplication(c *tc.C) {
	appUUID := s.createCAASApplication(c, "foo", life.Alive)
	_, err := s.DB().ExecContext(c.Context(), `
UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = (
    SELECT a.charm_uuid FROM application AS a WHERE a.uuid = ?
)`, appUUID)
	c.Assert(err, tc.ErrorIsNil)
	err = s.upsertK8sService(c, "foo", "provider", nil)
	c.Assert(err, tc.ErrorMatches, "cannot upsert cloud service for synthetic application")
	var count int
	c.Assert(s.DB().QueryRowContext(c.Context(), "SELECT COUNT(*) FROM k8s_service").Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *applicationStateSuite) TestK8sServiceFailedReplacementIsAtomic(c *tc.C) {
	s.createCAASApplication(c, "foo", life.Alive)
	c.Assert(s.upsertK8sService(c, "foo", "provider", network.ProviderAddresses{
		network.NewMachineAddress("lb.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress(),
	}), tc.ErrorIsNil)
	err := s.upsertK8sService(c, "foo", "replacement-provider", network.ProviderAddresses{{
		MachineAddress: network.MachineAddress{Value: "bad", Type: network.AddressType("invalid")},
	}})
	c.Assert(err, tc.NotNil)
	c.Check(s.k8sServiceAddresses(c, "foo"), tc.SameContents, []string{"lb.example.com|public"})
	var providerID string
	c.Assert(s.DB().QueryRowContext(c.Context(), "SELECT provider_id FROM k8s_service").Scan(&providerID), tc.ErrorIsNil)
	c.Check(providerID, tc.Equals, "provider")
}

func (s *applicationStateSuite) TestK8sServiceHostnameChangeNotifications(c *tc.C) {
	s.createCAASApplication(c, "foo", life.Alive)
	_, err := s.DB().ExecContext(c.Context(), "DELETE FROM change_log")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.upsertK8sService(c, "foo", "provider", network.ProviderAddresses{
		network.NewMachineAddress("lb.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress(),
	}), tc.ErrorIsNil)
	for _, namespace := range []string{"k8s_service", "fqdn_address", "net_node_fqdn_address"} {
		var count int
		err := s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM change_log AS cl
JOIN change_log_namespace AS ns ON ns.id = cl.namespace_id
WHERE ns.namespace = ? AND cl.edit_type_id = 1`, namespace).Scan(&count)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(count, tc.Equals, 1, tc.Commentf("namespace %s", namespace))
	}
	_, err = s.DB().ExecContext(c.Context(), "UPDATE fqdn_address SET scope_id = 1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.upsertK8sService(c, "foo", "provider", nil), tc.ErrorIsNil)
	for _, event := range []struct {
		namespace string
		edit      int
	}{
		{"fqdn_address", 2}, {"fqdn_address", 4}, {"net_node_fqdn_address", 4},
	} {
		var count int
		err := s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM change_log AS cl
JOIN change_log_namespace AS ns ON ns.id = cl.namespace_id
WHERE ns.namespace = ? AND cl.edit_type_id = ?`, event.namespace, event.edit).Scan(&count)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(count, tc.Equals, 1)
	}
}

func (s *applicationStateSuite) k8sServiceAddresses(c *tc.C, appName string) []string {
	rows, err := s.DB().QueryContext(c.Context(), `
SELECT ia.address_value, scope.name
FROM application AS a
JOIN k8s_service AS ks ON ks.application_uuid = a.uuid
JOIN ip_address AS ia ON ia.net_node_uuid = ks.net_node_uuid
JOIN ip_address_scope AS scope ON scope.id = ia.scope_id
WHERE a.name = ?
UNION ALL
SELECT fa.address, scope.name
FROM application AS a
JOIN k8s_service AS ks ON ks.application_uuid = a.uuid
JOIN net_node_fqdn_address AS nnfa ON nnfa.net_node_uuid = ks.net_node_uuid
JOIN fqdn_address AS fa ON fa.uuid = nnfa.address_uuid
JOIN network_address_scope AS scope ON scope.id = fa.scope_id
WHERE a.name = ?`, appName, appName)
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()
	var result []string
	for rows.Next() {
		var address, scope string
		c.Assert(rows.Scan(&address, &scope), tc.ErrorIsNil)
		result = append(result, address+"|"+scope)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
	return result
}
