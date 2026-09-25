// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v12"
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
		setup    func() []description.RemoteApplication
		expected func(c *tc.C, remoteApps map[string]RemoteApplicationOfferer)
	}{
		{
			name: "no remote applications",
			setup: func() []description.RemoteApplication {
				return nil
			},
			expected: func(c *tc.C, remoteApps map[string]RemoteApplicationOfferer) {
				c.Check(remoteApps, tc.HasLen, 0)
			},
		},
		{
			name: "no relations",
			setup: func() []description.RemoteApplication {
				m := description.NewModel(description.ModelArgs{})
				remoteApp := m.AddRemoteApplication(description.RemoteApplicationArgs{
					Name:            "remote-consumer-1",
					IsConsumerProxy: true,
					OfferUUID:       "foo",
					SourceModelUUID: "bar",
				})
				return []description.RemoteApplication{remoteApp}
			},
			expected: func(c *tc.C, remoteApps map[string]RemoteApplicationOfferer) {
				c.Check(remoteApps, tc.HasLen, 0)
			},
		},
		{
			name: "consumer proxy remote application",
			setup: func() []description.RemoteApplication {
				m := description.NewModel(description.ModelArgs{})

				remoteApp0 := m.AddRemoteApplication(description.RemoteApplicationArgs{
					Name:            "remote-consumer",
					IsConsumerProxy: true,
					OfferUUID:       "foo",
					SourceModelUUID: "bar",
				})

				return []description.RemoteApplication{remoteApp0}
			},
			expected: func(c *tc.C, remoteApps map[string]RemoteApplicationOfferer) {
				c.Check(remoteApps, tc.HasLen, 0)
			},
		},
		{
			name: "remote application",
			setup: func() []description.RemoteApplication {
				m := description.NewModel(description.ModelArgs{})

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

				return []description.RemoteApplication{remoteApp0}
			},
			expected: func(c *tc.C, remoteApps map[string]RemoteApplicationOfferer) {
				c.Assert(remoteApps, tc.HasLen, 1)

				remoteApp, ok := remoteApps["deadbeef"]
				c.Assert(ok, tc.IsTrue)
				c.Assert(remoteApp.IsEmpty(), tc.IsFalse)

				c.Check(remoteApp.Primary.Name(), tc.Equals, "dummy-source")
				c.Check(remoteApp.Primary.SourceModelUUID(), tc.Equals, "bar")
			},
		},
		{
			name: "duplicate remote application with relations",
			setup: func() []description.RemoteApplication {
				m := description.NewModel(description.ModelArgs{})

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

				return []description.RemoteApplication{remoteApp0, remoteApp1}
			},
			expected: func(c *tc.C, remoteApps map[string]RemoteApplicationOfferer) {
				c.Assert(remoteApps, tc.HasLen, 1)

				remoteApp, ok := remoteApps["deadbeef"]
				c.Assert(ok, tc.IsTrue)
				c.Assert(remoteApp.IsEmpty(), tc.IsFalse)

				c.Check(remoteApp.Primary.Name(), tc.Equals, "foo")
				c.Check(remoteApp.Primary.SourceModelUUID(), tc.Equals, "bar")

				c.Assert(remoteApp.Duplicates, tc.HasLen, 1)

				c.Check(remoteApp.Duplicates[0].Name(), tc.Equals, "baz")
				c.Check(remoteApp.Duplicates[0].SourceModelUUID(), tc.Equals, "bar")
			},
		},
		{
			name: "duplicate remote application with relations - inverted endpoints",
			setup: func() []description.RemoteApplication {
				m := description.NewModel(description.ModelArgs{})

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

				return []description.RemoteApplication{remoteApp0, remoteApp1}
			},
			expected: func(c *tc.C, remoteApps map[string]RemoteApplicationOfferer) {
				c.Assert(remoteApps, tc.HasLen, 1)

				remoteApp, ok := remoteApps["deadbeef"]
				c.Assert(ok, tc.IsTrue)
				c.Assert(remoteApp.IsEmpty(), tc.IsFalse)

				c.Check(remoteApp.Primary.Name(), tc.Equals, "foo")
				c.Check(remoteApp.Primary.SourceModelUUID(), tc.Equals, "bar")

				c.Assert(remoteApp.Duplicates, tc.HasLen, 1)

				c.Check(remoteApp.Duplicates[0].Name(), tc.Equals, "baz")
				c.Check(remoteApp.Duplicates[0].SourceModelUUID(), tc.Equals, "bar")
			},
		},
	}

	for _, test := range tests {
		c.Logf("Test case: %s", test.name)

		remoteApps := test.setup()
		result, err := UniqueRemoteOfferApplications(remoteApps)

		c.Assert(err, tc.IsNil)
		test.expected(c, result)
	}
}

func (s *relationSuite) TestUniqueRemoteOfferApplicationsInvalidSourceModelUUID(c *tc.C) {
	m := description.NewModel(description.ModelArgs{})

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

	_, err := UniqueRemoteOfferApplications([]description.RemoteApplication{remoteApp0, remoteApp1})
	c.Assert(err, tc.ErrorMatches, "multiple remote application offerers with the same offer UUID.*but different source model UUIDs.*")
}

func (s *relationSuite) TestUniqueRemoteOfferApplicationsInvalidEndpoints(c *tc.C) {
	m := description.NewModel(description.ModelArgs{})

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

	_, err := UniqueRemoteOfferApplications([]description.RemoteApplication{remoteApp0, remoteApp1})
	c.Assert(err, tc.ErrorMatches, "multiple remote application offerers with the same offer UUID.*but different endpoints.*")
}

// TestRewriteUnitName checks the unit name rewrite rule shared by the
// relation and crossmodelrelation domain imports: a unit name is re-written
// onto a new application name only when its application segment matches the
// old name exactly, so a prefix match cannot rename another application's
// units, and names that are not valid unit names are an error.
func (s *relationSuite) TestRewriteUnitName(c *tc.C) {
	tests := []struct {
		name               string
		unitName           string
		oldApplicationName string
		newApplicationName string
		expected           string
		expectedErr        string
	}{
		{
			name:               "unit name belonging to the old application",
			unitName:           "second/0",
			oldApplicationName: "second",
			newApplicationName: "first",
			expected:           "first/0",
		},
		{
			name:               "unit name of an application sharing a prefix",
			unitName:           "secondaire/0",
			oldApplicationName: "second",
			newApplicationName: "first",
			expected:           "secondaire/0",
		},
		{
			name:               "unit name belonging to another application",
			unitName:           "other/0",
			oldApplicationName: "second",
			newApplicationName: "first",
			expected:           "other/0",
		},
		{
			name:               "name without a unit number",
			unitName:           "second",
			oldApplicationName: "second",
			newApplicationName: "first",
			expectedErr:        `parsing unit name "second": invalid unit name: "second"`,
		},
		{
			name:               "empty name",
			unitName:           "",
			oldApplicationName: "second",
			newApplicationName: "first",
			expectedErr:        `parsing unit name "": invalid unit name: ""`,
		},
		{
			name:               "invalid new application name",
			unitName:           "second/0",
			oldApplicationName: "second",
			newApplicationName: "First",
			expectedErr:        `rewriting unit name "second/0": invalid unit name: "First/0"`,
		},
	}

	for _, test := range tests {
		c.Logf("Test case: %s", test.name)

		result, err := RewriteUnitName(
			test.unitName, test.oldApplicationName, test.newApplicationName)

		if test.expectedErr != "" {
			c.Assert(err, tc.ErrorMatches, test.expectedErr)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.Equals, test.expected)
	}
}

// TestRemoteApplicationOffererRewriteUnitName checks re-writing unit names
// belonging to the primary remote application or any of the offerer's
// duplicate aliases onto the primary application name; names that are not
// valid unit names are an error.
func (s *relationSuite) TestRemoteApplicationOffererRewriteUnitName(c *tc.C) {
	m := description.NewModel(description.ModelArgs{})
	offerer := RemoteApplicationOfferer{
		Primary: m.AddRemoteApplication(description.RemoteApplicationArgs{Name: "first"}),
		Duplicates: []description.RemoteApplication{
			m.AddRemoteApplication(description.RemoteApplicationArgs{Name: "second"}),
			m.AddRemoteApplication(description.RemoteApplicationArgs{Name: "third"}),
		},
	}

	tests := []struct {
		name        string
		unitName    string
		expected    string
		expectedErr string
	}{
		{
			name:     "unit name belonging to the primary",
			unitName: "first/0",
			expected: "first/0",
		},
		{
			name:     "unit name belonging to the first alias",
			unitName: "second/0",
			expected: "first/0",
		},
		{
			name:     "unit name belonging to the second alias",
			unitName: "third/7",
			expected: "first/7",
		},
		{
			name:     "unit name of an application sharing a prefix",
			unitName: "secondaire/0",
			expected: "secondaire/0",
		},
		{
			name:        "name without a unit number",
			unitName:    "second",
			expectedErr: `parsing unit name "second": invalid unit name: "second"`,
		},
	}

	for _, test := range tests {
		c.Logf("Test case: %s", test.name)

		result, err := offerer.RewriteUnitName(test.unitName)

		if test.expectedErr != "" {
			c.Assert(err, tc.ErrorMatches, test.expectedErr)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.Equals, test.expected)
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
