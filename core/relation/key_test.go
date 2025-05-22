// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
)

type relationKeySuite struct {
	testhelpers.IsolationSuite
}

func TestRelationKeySuite(t *testing.T) {
	tc.Run(t, &relationKeySuite{})
}

func (s *relationKeySuite) TestParseRelationKey(c *tc.C) {
	tests := []struct {
		summary     string
		keyString   string
		expectedKey Key
	}{{
		summary:   "regular relation",
		keyString: "app1:end1 app2:end2",
		expectedKey: []EndpointIdentifier{{
			ApplicationName: "app1",
			EndpointName:    "end1",
			Role:            charm.RoleRequirer,
		}, {
			ApplicationName: "app2",
			EndpointName:    "end2",
			Role:            charm.RoleProvider,
		}},
	}, {
		summary:   "peer relation",
		keyString: "app:end",
		expectedKey: []EndpointIdentifier{{
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}},
	}, {
		summary:   "regular relation with similar endpoints",
		keyString: "app_1:end app_2:end",
		expectedKey: []EndpointIdentifier{{
			ApplicationName: "app_1",
			EndpointName:    "end",
			Role:            charm.RoleRequirer,
		}, {
			ApplicationName: "app_2",
			EndpointName:    "end",
			Role:            charm.RoleProvider,
		}},
	}}

	count := len(tests)
	for i, test := range tests {
		c.Logf("test %d of %d: %s", count, i+1, test.summary)
		key, err := NewKeyFromString(test.keyString)
		c.Check(err, tc.ErrorIsNil)
		c.Check(key, tc.DeepEquals, test.expectedKey)
		// Check a string can be turned to a key and back.
		c.Check(key.String(), tc.Equals, test.keyString)
	}
}

func (s *relationKeySuite) TestNewKey(c *tc.C) {
	tests := []struct {
		summary             string
		endpointIdentifiers []EndpointIdentifier
		expectedKey         Key
	}{{
		summary: "regular relation with endpoints in correct order",
		endpointIdentifiers: []EndpointIdentifier{{
			ApplicationName: "app1",
			EndpointName:    "end1",
			Role:            charm.RoleRequirer,
		}, {
			ApplicationName: "app2",
			EndpointName:    "end2",
			Role:            charm.RoleProvider,
		}},
		expectedKey: []EndpointIdentifier{{
			ApplicationName: "app1",
			EndpointName:    "end1",
			Role:            charm.RoleRequirer,
		}, {
			ApplicationName: "app2",
			EndpointName:    "end2",
			Role:            charm.RoleProvider,
		}},
	}, {
		summary: "regular relation with endpoints in wrong order",
		endpointIdentifiers: []EndpointIdentifier{{
			ApplicationName: "app2",
			EndpointName:    "end2",
			Role:            charm.RoleProvider,
		}, {
			ApplicationName: "app1",
			EndpointName:    "end1",
			Role:            charm.RoleRequirer,
		}},
		expectedKey: []EndpointIdentifier{{
			ApplicationName: "app1",
			EndpointName:    "end1",
			Role:            charm.RoleRequirer,
		}, {
			ApplicationName: "app2",
			EndpointName:    "end2",
			Role:            charm.RoleProvider,
		}},
	}, {
		summary: "peer relation",
		endpointIdentifiers: []EndpointIdentifier{{
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}},
		expectedKey: []EndpointIdentifier{{
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}},
	}, {
		summary: "regular relation with similar endpoints",
		endpointIdentifiers: []EndpointIdentifier{{
			ApplicationName: "app_1",
			EndpointName:    "end",
			Role:            charm.RoleRequirer,
		}, {
			ApplicationName: "app_2",
			EndpointName:    "end",
			Role:            charm.RoleProvider,
		}},
		expectedKey: []EndpointIdentifier{{
			ApplicationName: "app_1",
			EndpointName:    "end",
			Role:            charm.RoleRequirer,
		}, {
			ApplicationName: "app_2",
			EndpointName:    "end",
			Role:            charm.RoleProvider,
		}},
	}}

	count := len(tests)
	for i, test := range tests {
		c.Logf("test %d of %d: %s", count, i+1, test.summary)
		key, err := NewKey(test.endpointIdentifiers)
		c.Check(err, tc.ErrorIsNil)
		c.Check(key, tc.DeepEquals, test.expectedKey)
	}
}

func (s *relationKeySuite) TestNewKeyError(c *tc.C) {
	tests := []struct {
		summary             string
		endpointIdentifiers []EndpointIdentifier
		errorRegex          string
	}{{
		summary: "double requirer",
		endpointIdentifiers: []EndpointIdentifier{{
			ApplicationName: "app1",
			EndpointName:    "end1",
			Role:            charm.RoleProvider,
		}, {
			ApplicationName: "app2",
			EndpointName:    "end2",
			Role:            charm.RoleProvider,
		}},
		errorRegex: `two endpoints provided, expected roles "provider" and "requirer", got: "provider" and "provider"`,
	}, {
		summary: "double peer relation",
		endpointIdentifiers: []EndpointIdentifier{{
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}, {
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}},
		errorRegex: `two endpoints provided, expected roles "provider" and "requirer", got: "peer" and "peer"`,
	}, {
		summary: "single requirer",
		endpointIdentifiers: []EndpointIdentifier{{
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RoleRequirer,
		}},
		errorRegex: `one endpoint provided, expected role "peer", got: "requirer"`,
	}, {
		summary:             "nil list",
		endpointIdentifiers: nil,
		errorRegex:          `expected 1 or 2 endpoint identifiers, got 0`,
	}, {
		summary: "triple list",
		endpointIdentifiers: []EndpointIdentifier{{
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}, {
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}, {
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}},
		errorRegex: `expected 1 or 2 endpoint identifiers, got 3`,
	}}

	count := len(tests)
	for i, test := range tests {
		c.Logf("test %d of %d: %s", count, i+1, test.summary)
		_, err := NewKey(test.endpointIdentifiers)
		c.Check(err, tc.ErrorMatches, test.errorRegex)
	}
}

func (s *relationKeySuite) TestValidate(c *tc.C) {
	tests := []struct {
		summary string
		key     Key
	}{{
		summary: "regular relation with endpoints in correct order",
		key: []EndpointIdentifier{{
			ApplicationName: "app1",
			EndpointName:    "end1",
			Role:            charm.RoleRequirer,
		}, {
			ApplicationName: "app2",
			EndpointName:    "end2",
			Role:            charm.RoleProvider,
		}},
	}, {
		summary: "regular relation with endpoints in wrong order",
		key: []EndpointIdentifier{{
			ApplicationName: "app1",
			EndpointName:    "end1",
			Role:            charm.RoleRequirer,
		}, {
			ApplicationName: "app2",
			EndpointName:    "end2",
			Role:            charm.RoleProvider,
		}},
	}, {
		summary: "peer relation",
		key: []EndpointIdentifier{{
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}},
	}, {
		summary: "regular relation with similar endpoints",
		key: []EndpointIdentifier{{
			ApplicationName: "app_1",
			EndpointName:    "end",
			Role:            charm.RoleRequirer,
		}, {
			ApplicationName: "app_2",
			EndpointName:    "end",
			Role:            charm.RoleProvider,
		}},
	}}

	count := len(tests)
	for i, test := range tests {
		c.Logf("test %d of %d: %s", count, i+1, test.summary)
		c.Check(test.key.Validate(), tc.ErrorIsNil)
	}
}

func (s *relationKeySuite) TestValidateError(c *tc.C) {
	tests := []struct {
		summary string
		key     Key
	}{{
		summary: "double requirer",
		key: []EndpointIdentifier{{
			ApplicationName: "app1",
			EndpointName:    "end1",
			Role:            charm.RoleProvider,
		}, {
			ApplicationName: "app2",
			EndpointName:    "end2",
			Role:            charm.RoleProvider,
		}},
	}, {
		summary: "double peer relation",
		key: []EndpointIdentifier{{
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}, {
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}},
	}, {
		summary: "single requirer",
		key: []EndpointIdentifier{{
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RoleRequirer,
		}},
	}, {
		summary: "nil list",
		key:     nil,
	}, {
		summary: "triple list",
		key: []EndpointIdentifier{{
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}, {
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}, {
			ApplicationName: "app",
			EndpointName:    "end",
			Role:            charm.RolePeer,
		}},
	}}

	count := len(tests)
	for i, test := range tests {
		c.Logf("test %d of %d: %s", count, i+1, test.summary)
		err := test.key.Validate()
		c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	}
}

func (s *relationKeySuite) TestParseRelationKeyError(c *tc.C) {
	tests := []struct {
		summary    string
		keyString  string
		errorRegex string
	}{{
		summary:    "too many endpoints in string",
		keyString:  "app1:end1 app2:end2 app3:end3",
		errorRegex: "expected 1 or 2 endpoints in relation key, got 3.*",
	}, {
		summary:    "empty string",
		keyString:  "",
		errorRegex: "expected 1 or 2 endpoints in relation key, got 0.*",
	}, {
		summary:    "no space",
		keyString:  "app_1:end_1app2:end2",
		errorRegex: ".* expected endpoint string of the form <application-name>:<endpoint-name>, got.*",
	}}

	count := len(tests)
	for i, test := range tests {
		c.Logf("test %d of %d: %s", count, i+1, test.summary)
		_, err := NewKeyFromString(test.keyString)
		c.Check(err, tc.ErrorMatches, test.errorRegex)
	}
}

func (*relationKeySuite) TestParseKeyFromTagString(c *tc.C) {
	relationTag := names.NewRelationTag("mysql:database wordpress:mysql")
	key, err := ParseKeyFromTagString(relationTag.String())
	c.Assert(err, tc.IsNil)
	c.Check(key, tc.DeepEquals, Key([]EndpointIdentifier{{
		ApplicationName: "mysql",
		EndpointName:    "database",
		Role:            "requirer",
	}, {
		ApplicationName: "wordpress",
		EndpointName:    "mysql",
		Role:            "provider",
	}},
	))
}

func (*relationKeySuite) TestParseKeyFromTagStringFails(c *tc.C) {
	unitTag := names.NewUnitTag("mysql/0")
	_, err := ParseKeyFromTagString(unitTag.String())
	c.Check(err, tc.ErrorMatches, `"unit-mysql-0" is not a valid relation tag`)

	_, err = ParseKeyFromTagString("")
	c.Check(err, tc.ErrorMatches, `"" is not a valid tag`)
}
