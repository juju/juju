// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v11"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type relationSuite struct {
	testhelpers.IsolationSuite
}

func TestRelationSuite(t *testing.T) {
	tc.Run(t, &relationSuite{})
}

func (s *relationSuite) TestMatchRelationEndpointByApplications(c *tc.C) {

	tests := []struct {
		name     string
		setup    func() (description.Relation, []description.RemoteApplication)
		expected bool
	}{
		{
			name: "no remote applications",
			setup: func() (description.Relation, []description.RemoteApplication) {
				m := description.NewModel(description.ModelArgs{})
				relation := m.AddRelation(description.RelationArgs{
					Id:  0,
					Key: "foo:sink",
				})
				relation.AddEndpoint(description.EndpointArgs{
					ApplicationName: "app",
					Name:            "endpoint",
				})

				return relation, nil
			},
			expected: false,
		},
		{
			name: "is not consumer proxy remote application",
			setup: func() (description.Relation, []description.RemoteApplication) {
				m := newModel()
				relation := m.AddRelation(description.RelationArgs{
					Id:  0,
					Key: "foo:sink",
				})
				relation.AddEndpoint(description.EndpointArgs{
					ApplicationName: "remote-non-consumer",
					Name:            "endpoint",
				})
				return relation, m.RemoteApplications()
			},
			expected: false,
		},
		{
			name: "is consumer proxy remote application",
			setup: func() (description.Relation, []description.RemoteApplication) {
				m := newModel()
				relation := m.AddRelation(description.RelationArgs{
					Id:  0,
					Key: "foo:sink",
				})
				relation.AddEndpoint(description.EndpointArgs{
					ApplicationName: "remote-consumer",
					Name:            "endpoint",
				})
				return relation, m.RemoteApplications()
			},
			expected: true,
		},
	}

	for _, test := range tests {
		c.Logf("Test case: %s", test.name)

		relation, remoteApps := test.setup()
		result := ContainsRelationEndpointApplicationName(relation, GetUniqueRemoteConsumersNames(remoteApps))

		c.Assert(result, tc.Equals, test.expected)
	}
}

func newModel() description.Model {
	m := description.NewModel(description.ModelArgs{})

	m.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-consumer",
		IsConsumerProxy: true,
		OfferUUID:       "foo",
		SourceModelUUID: "bar",
	})
	m.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-non-consumer",
		IsConsumerProxy: false,
		OfferUUID:       "foo",
		SourceModelUUID: "bar",
	})

	return m
}
