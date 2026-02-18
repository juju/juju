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

func (s *relationSuite) TestUniqueRemoteOfferApplications(c *tc.C) {
	tests := []struct {
		name     string
		setup    func() ([]description.RemoteApplication, []description.Relation)
		expected func(c *tc.C, remoteApps map[string][]description.RemoteApplication)
	}{
		{
			name: "no remote applications",
			setup: func() ([]description.RemoteApplication, []description.Relation) {
				return nil, nil
			},
			expected: func(c *tc.C, remoteApps map[string][]description.RemoteApplication) {
				c.Check(remoteApps, tc.HasLen, 0)
			},
		},
		{
			name: "no relations",
			setup: func() ([]description.RemoteApplication, []description.Relation) {
				m := description.NewModel(description.ModelArgs{})
				remoteApp := m.AddRemoteApplication(description.RemoteApplicationArgs{
					Name:            "remote-consumer-1",
					IsConsumerProxy: true,
					OfferUUID:       "foo",
					SourceModelUUID: "bar",
				})
				return []description.RemoteApplication{remoteApp}, nil
			},
			expected: func(c *tc.C, remoteApps map[string][]description.RemoteApplication) {
				c.Check(remoteApps, tc.HasLen, 0)
			},
		},
		{
			name: "consumer proxy remote application",
			setup: func() ([]description.RemoteApplication, []description.Relation) {
				m := description.NewModel(description.ModelArgs{})
				relation := m.AddRelation(description.RelationArgs{
					Id:  0,
					Key: "foo:sink",
				})
				relation.AddEndpoint(description.EndpointArgs{
					ApplicationName: "remote-consumer",
					Name:            "endpoint",
				})

				remoteApp0 := m.AddRemoteApplication(description.RemoteApplicationArgs{
					Name:            "remote-consumer",
					IsConsumerProxy: true,
					OfferUUID:       "foo",
					SourceModelUUID: "bar",
				})

				return []description.RemoteApplication{remoteApp0},
					[]description.Relation{relation}
			},
			expected: func(c *tc.C, remoteApps map[string][]description.RemoteApplication) {
				c.Check(remoteApps, tc.HasLen, 0)
			},
		},
		{
			name: "remote application",
			setup: func() ([]description.RemoteApplication, []description.Relation) {
				m := description.NewModel(description.ModelArgs{})
				relation := m.AddRelation(description.RelationArgs{
					Id:  0,
					Key: "dummy-source:sink dummy-sink:source",
				})
				relation.AddEndpoint(description.EndpointArgs{
					ApplicationName: "dummy-source",
					Name:            "sink",
				})

				remoteApp0 := m.AddRemoteApplication(description.RemoteApplicationArgs{
					Name:            "dummy-source",
					OfferUUID:       "deadbeef",
					SourceModelUUID: "bar",
				})
				remoteApp0.AddEndpoint(description.RemoteEndpointArgs{
					Name:      "dummy-source",
					Interface: "dummy-token",
					Role:      "requirer",
				})

				return []description.RemoteApplication{remoteApp0},
					[]description.Relation{relation}
			},
			expected: func(c *tc.C, remoteApps map[string][]description.RemoteApplication) {
				c.Assert(remoteApps, tc.HasLen, 1)

				remoteApp, ok := remoteApps["deadbeef"]
				c.Assert(ok, tc.IsTrue)
				c.Assert(remoteApp, tc.HasLen, 1)

				c.Check(remoteApp[0].Name(), tc.Equals, "dummy-source")
				c.Check(remoteApp[0].SourceModelUUID(), tc.Equals, "bar")
			},
		},
		{
			name: "duplicate remote application with one linked to a relation",
			setup: func() ([]description.RemoteApplication, []description.Relation) {
				m := description.NewModel(description.ModelArgs{})
				relation := m.AddRelation(description.RelationArgs{
					Id:  0,
					Key: "foo:sink dummy-sink:source",
				})
				relation.AddEndpoint(description.EndpointArgs{
					ApplicationName: "foo",
					Name:            "sink",
				})
				relation.AddEndpoint(description.EndpointArgs{
					ApplicationName: "dummy-sink",
					Name:            "source",
				})

				remoteApp0 := m.AddRemoteApplication(description.RemoteApplicationArgs{
					Name:            "foo",
					OfferUUID:       "deadbeef",
					SourceModelUUID: "bar",
				})
				remoteApp0.AddEndpoint(description.RemoteEndpointArgs{
					Name:      "dummy-source",
					Interface: "dummy-token",
					Role:      "requirer",
				})

				remoteApp1 := m.AddRemoteApplication(description.RemoteApplicationArgs{
					Name:            "bar",
					OfferUUID:       "deadbeef",
					SourceModelUUID: "bar",
				})
				remoteApp1.AddEndpoint(description.RemoteEndpointArgs{
					Name:      "dummy-source",
					Interface: "dummy-token",
					Role:      "requirer",
				})

				return []description.RemoteApplication{remoteApp0, remoteApp1},
					[]description.Relation{relation}
			},
			expected: func(c *tc.C, remoteApps map[string][]description.RemoteApplication) {
				c.Assert(remoteApps, tc.HasLen, 1)

				remoteApp, ok := remoteApps["deadbeef"]
				c.Assert(ok, tc.IsTrue)
				c.Assert(remoteApp, tc.HasLen, 1)

				c.Check(remoteApp[0].Name(), tc.Equals, "foo")
				c.Check(remoteApp[0].SourceModelUUID(), tc.Equals, "bar")
			},
		},
		{
			name: "duplicate remote application with relations",
			setup: func() ([]description.RemoteApplication, []description.Relation) {
				m := description.NewModel(description.ModelArgs{})
				relation0 := m.AddRelation(description.RelationArgs{
					Id:  0,
					Key: "foo:sink dummy-sink:source",
				})
				relation0.AddEndpoint(description.EndpointArgs{
					ApplicationName: "foo",
					Name:            "sink",
				})
				relation0.AddEndpoint(description.EndpointArgs{
					ApplicationName: "dummy-sink",
					Name:            "source",
				})

				relation1 := m.AddRelation(description.RelationArgs{
					Id:  1,
					Key: "baz:sink dummy-sink:source",
				})
				relation1.AddEndpoint(description.EndpointArgs{
					ApplicationName: "baz",
					Name:            "sink",
				})
				relation1.AddEndpoint(description.EndpointArgs{
					ApplicationName: "dummy-sink",
					Name:            "source",
				})

				remoteApp0 := m.AddRemoteApplication(description.RemoteApplicationArgs{
					Name:            "foo",
					OfferUUID:       "deadbeef",
					SourceModelUUID: "bar",
				})
				remoteApp0.AddEndpoint(description.RemoteEndpointArgs{
					Name:      "dummy-source",
					Interface: "dummy-token",
					Role:      "requirer",
				})

				remoteApp1 := m.AddRemoteApplication(description.RemoteApplicationArgs{
					Name:            "baz",
					OfferUUID:       "deadbeef",
					SourceModelUUID: "bar",
				})
				remoteApp1.AddEndpoint(description.RemoteEndpointArgs{
					Name:      "dummy-source",
					Interface: "dummy-token",
					Role:      "requirer",
				})

				return []description.RemoteApplication{remoteApp0, remoteApp1},
					[]description.Relation{relation0, relation1}
			},
			expected: func(c *tc.C, remoteApps map[string][]description.RemoteApplication) {
				c.Assert(remoteApps, tc.HasLen, 1)

				remoteApp, ok := remoteApps["deadbeef"]
				c.Assert(ok, tc.IsTrue)
				c.Assert(remoteApp, tc.HasLen, 2)

				c.Check(remoteApp[0].Name(), tc.Equals, "foo")
				c.Check(remoteApp[0].SourceModelUUID(), tc.Equals, "bar")

				c.Check(remoteApp[1].Name(), tc.Equals, "baz")
				c.Check(remoteApp[1].SourceModelUUID(), tc.Equals, "bar")
			},
		},
		{
			name: "duplicate remote application with relations - inverted endpoints",
			setup: func() ([]description.RemoteApplication, []description.Relation) {
				m := description.NewModel(description.ModelArgs{})
				relation0 := m.AddRelation(description.RelationArgs{
					Id:  0,
					Key: "foo:sink dummy-sink:source",
				})
				relation0.AddEndpoint(description.EndpointArgs{
					ApplicationName: "dummy-sink",
					Name:            "source",
				})
				relation0.AddEndpoint(description.EndpointArgs{
					ApplicationName: "foo",
					Name:            "sink",
				})

				relation1 := m.AddRelation(description.RelationArgs{
					Id:  1,
					Key: "baz:sink dummy-sink:source",
				})

				relation1.AddEndpoint(description.EndpointArgs{
					ApplicationName: "dummy-sink",
					Name:            "source",
				})
				relation1.AddEndpoint(description.EndpointArgs{
					ApplicationName: "baz",
					Name:            "sink",
				})

				remoteApp0 := m.AddRemoteApplication(description.RemoteApplicationArgs{
					Name:            "foo",
					OfferUUID:       "deadbeef",
					SourceModelUUID: "bar",
				})
				remoteApp0.AddEndpoint(description.RemoteEndpointArgs{
					Name:      "dummy-source",
					Interface: "dummy-token",
					Role:      "requirer",
				})

				remoteApp1 := m.AddRemoteApplication(description.RemoteApplicationArgs{
					Name:            "baz",
					OfferUUID:       "deadbeef",
					SourceModelUUID: "bar",
				})
				remoteApp1.AddEndpoint(description.RemoteEndpointArgs{
					Name:      "dummy-source",
					Interface: "dummy-token",
					Role:      "requirer",
				})

				return []description.RemoteApplication{remoteApp0, remoteApp1},
					[]description.Relation{relation0, relation1}
			},
			expected: func(c *tc.C, remoteApps map[string][]description.RemoteApplication) {
				c.Assert(remoteApps, tc.HasLen, 1)

				remoteApp, ok := remoteApps["deadbeef"]
				c.Assert(ok, tc.IsTrue)
				c.Assert(remoteApp, tc.HasLen, 2)

				c.Check(remoteApp[0].Name(), tc.Equals, "foo")
				c.Check(remoteApp[0].SourceModelUUID(), tc.Equals, "bar")

				c.Check(remoteApp[1].Name(), tc.Equals, "baz")
				c.Check(remoteApp[1].SourceModelUUID(), tc.Equals, "bar")
			},
		},
	}

	for _, test := range tests {
		c.Logf("Test case: %s", test.name)

		remoteApps, relations := test.setup()
		result, err := UniqueRemoteOfferApplications(remoteApps, relations)

		c.Assert(err, tc.IsNil)
		test.expected(c, result)
	}
}

func (s *relationSuite) TestUniqueRemoteOfferApplicationsInvalidSourceModelUUID(c *tc.C) {
	m := description.NewModel(description.ModelArgs{})
	relation0 := m.AddRelation(description.RelationArgs{
		Id:  0,
		Key: "foo:sink dummy-sink:source",
	})
	relation0.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-sink",
		Name:            "source",
	})
	relation0.AddEndpoint(description.EndpointArgs{
		ApplicationName: "foo",
		Name:            "sink",
	})

	relation1 := m.AddRelation(description.RelationArgs{
		Id:  1,
		Key: "baz:sink dummy-sink:source",
	})

	relation1.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-sink",
		Name:            "source",
	})
	relation1.AddEndpoint(description.EndpointArgs{
		ApplicationName: "baz",
		Name:            "sink",
	})

	remoteApp0 := m.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "foo",
		OfferUUID:       "deadbeef",
		SourceModelUUID: "bar",
	})
	remoteApp0.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "dummy-source",
		Interface: "dummy-token",
		Role:      "requirer",
	})

	remoteApp1 := m.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "baz",
		OfferUUID:       "deadbeef",
		SourceModelUUID: "blah",
	})
	remoteApp1.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "dummy-source",
		Interface: "dummy-token",
		Role:      "requirer",
	})

	_, err := UniqueRemoteOfferApplications([]description.RemoteApplication{remoteApp0, remoteApp1}, []description.Relation{relation0, relation1})
	c.Assert(err, tc.ErrorMatches, "multiple remote application offerers with the same offer UUID.*but different source model UUIDs.*")
}

func (s *relationSuite) TestUniqueRemoteOfferApplicationsInvalidEndpoints(c *tc.C) {
	m := description.NewModel(description.ModelArgs{})
	relation0 := m.AddRelation(description.RelationArgs{
		Id:  0,
		Key: "foo:sink dummy-sink:source",
	})
	relation0.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-sink",
		Name:            "source",
	})
	relation0.AddEndpoint(description.EndpointArgs{
		ApplicationName: "foo",
		Name:            "sink",
	})

	relation1 := m.AddRelation(description.RelationArgs{
		Id:  1,
		Key: "baz:sink dummy-sink:source",
	})

	relation1.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-sink",
		Name:            "source",
	})
	relation1.AddEndpoint(description.EndpointArgs{
		ApplicationName: "baz",
		Name:            "sink",
	})

	remoteApp0 := m.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "foo",
		OfferUUID:       "deadbeef",
		SourceModelUUID: "bar",
	})
	remoteApp0.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "dummy-source",
		Interface: "dummy-token",
		Role:      "requirer",
	})

	remoteApp1 := m.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "baz",
		OfferUUID:       "deadbeef",
		SourceModelUUID: "bar",
	})
	remoteApp1.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "dummy-source",
		Interface: "dummy-token",
		Role:      "foo",
	})

	_, err := UniqueRemoteOfferApplications([]description.RemoteApplication{remoteApp0, remoteApp1}, []description.Relation{relation0, relation1})
	c.Assert(err, tc.ErrorMatches, "multiple remote application offerers with the same offer UUID.*but different endpoints.*")
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
