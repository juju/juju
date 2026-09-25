// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	domainmodelmigration "github.com/juju/juju/domain/modelmigration/modelmigration"
	"github.com/juju/juju/domain/relation"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// addOfferWithAliases adds a primary remote application and one or more
// duplicate aliases of the same offer to the model, returning the offerer
// used when importing relations that reference any of its aliases.
func addOfferWithAliases(
	m description.Model,
	primaryName string,
	aliasNames ...string,
) domainmodelmigration.RemoteApplicationOfferer {
	const (
		offerUUID       = "b9424bc3-1053-43a8-8386-c2faf56c4e9a"
		sourceModelUUID = "7e2421b5-4605-42d8-8636-bfcf7c35129f"
	)
	primary := m.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            primaryName,
		OfferUUID:       offerUUID,
		SourceModelUUID: sourceModelUUID,
	})
	primary.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "db",
		Role:      "provider",
		Interface: "dummy-token",
	})

	duplicates := make([]description.RemoteApplication, 0, len(aliasNames))
	for _, name := range aliasNames {
		duplicate := m.AddRemoteApplication(description.RemoteApplicationArgs{
			Name:            name,
			OfferUUID:       offerUUID,
			SourceModelUUID: sourceModelUUID,
		})
		duplicate.AddEndpoint(description.RemoteEndpointArgs{
			Name:      "db",
			Role:      "provider",
			Interface: "dummy-token",
		})
		duplicates = append(duplicates, duplicate)
	}

	return domainmodelmigration.RemoteApplicationOfferer{
		Primary:    primary,
		Duplicates: duplicates,
	}
}

// TestImportLegacyAliasPreservesRelationIdentityAndUnitSettings checks an
// established relation to the second alias of an offer. Deduplication must not
// replace its remote token, or leave settings addressed to a unit name that no
// longer matches the imported endpoint. Check the returned name so preserving
// the alias or consistently remapping it can both satisfy the contract.
func (s *importSuite) TestImportLegacyAliasPreservesRelationIdentityAndUnitSettings(c *tc.C) {
	m := description.NewModel(description.ModelArgs{})
	offerer := addOfferWithAliases(m, "first", "second")
	r := m.AddRelation(description.RelationArgs{Id: 42, Key: "client:db second:db"})
	r.AddEndpoint(description.EndpointArgs{ApplicationName: "client", Name: "db", Role: "requirer"})
	ep := r.AddEndpoint(description.EndpointArgs{ApplicationName: "second", Name: "db", Role: "provider"})
	ep.SetUnitSettings("second/0", map[string]any{"token": "keep-me"})
	key, err := corerelation.NewKeyFromString(r.Key())
	c.Assert(err, tc.ErrorIsNil)
	const token = "6049aa01-76c9-462d-8440-964a6e26aac2"
	arg, err := (&importOperation{}).createRemoteImportArg(r, offerer,
		[]relationRemoteEntity{{RelationKey: key, RelationUUID: token}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(arg.ID, tc.Equals, 42)
	c.Check(arg.UUID.String(), tc.Equals, token)
	c.Assert(arg.Endpoints, tc.HasLen, 2)
	c.Check(arg.Endpoints[1].UnitSettings[arg.Endpoints[1].ApplicationName+"/0"], tc.DeepEquals, map[string]any{"token": "keep-me"})
}

// TestImportLegacyAliasResolvesOwnTokenPerRelation mirrors a source model that
// holds a relation to each alias of the same offer. Both relation keys are
// re-written onto the primary name, so resolving the relation token from the
// re-written key would hand both relations the same UUID and abort the import
// with a "UNIQUE constraint failed: relation.uuid" error. Each relation must
// resolve its own token from the original key.
func (s *importSuite) TestImportLegacyAliasResolvesOwnTokenPerRelation(c *tc.C) {
	m := description.NewModel(description.ModelArgs{})
	offerer := addOfferWithAliases(m, "first", "second")

	relPrimary := m.AddRelation(description.RelationArgs{Id: 1, Key: "client:db first:db"})
	relPrimary.AddEndpoint(description.EndpointArgs{ApplicationName: "client", Name: "db", Role: "requirer"})
	relPrimary.AddEndpoint(description.EndpointArgs{ApplicationName: "first", Name: "db", Role: "provider"})

	relAlias := m.AddRelation(description.RelationArgs{Id: 2, Key: "client:db second:db"})
	relAlias.AddEndpoint(description.EndpointArgs{ApplicationName: "client", Name: "db", Role: "requirer"})
	epAlias := relAlias.AddEndpoint(description.EndpointArgs{ApplicationName: "second", Name: "db", Role: "provider"})
	epAlias.SetUnitSettings("second/0", map[string]any{"token": "keep-me"})

	const (
		tokenPrimary = "6049aa01-76c9-462d-8440-964a6e26aac2"
		tokenAlias   = "1d5b7a44-c7e5-4f9a-9b21-3d1f0e6c8a55"
	)
	keyPrimary, err := corerelation.NewKeyFromString(relPrimary.Key())
	c.Assert(err, tc.ErrorIsNil)
	keyAlias, err := corerelation.NewKeyFromString(relAlias.Key())
	c.Assert(err, tc.ErrorIsNil)
	remoteEntities := []relationRemoteEntity{
		{RelationKey: keyPrimary, RelationUUID: tokenPrimary},
		{RelationKey: keyAlias, RelationUUID: tokenAlias},
	}

	argPrimary, err := (&importOperation{}).createRemoteImportArg(relPrimary, offerer, remoteEntities)
	c.Assert(err, tc.ErrorIsNil)
	argAlias, err := (&importOperation{}).createRemoteImportArg(relAlias, offerer, remoteEntities)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(argPrimary.UUID.String(), tc.Equals, tokenPrimary)
	c.Check(argAlias.UUID.String(), tc.Equals, tokenAlias)

	// Both relations are re-written onto the primary application name.
	expectedKey, err := corerelation.NewKeyFromString("client:db first:db")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(argPrimary.Key, tc.DeepEquals, expectedKey)
	c.Check(argAlias.Key, tc.DeepEquals, expectedKey)

	// The alias relation's unit settings follow the endpoint rename.
	c.Assert(argAlias.Endpoints, tc.HasLen, 2)
	c.Check(argAlias.Endpoints[1].UnitSettings, tc.DeepEquals,
		map[string]map[string]any{"first/0": {"token": "keep-me"}})
}

// TestImportLegacyAliasFallbackUUIDWhenNoRemoteEntityMatches ensures that a
// relation without a matching remote entity token still imports, with a newly
// generated UUID, rather than failing.
func (s *importSuite) TestImportLegacyAliasFallbackUUIDWhenNoRemoteEntityMatches(c *tc.C) {
	m := description.NewModel(description.ModelArgs{})
	offerer := addOfferWithAliases(m, "first", "second")
	r := m.AddRelation(description.RelationArgs{Id: 3, Key: "client:db second:db"})
	r.AddEndpoint(description.EndpointArgs{ApplicationName: "client", Name: "db", Role: "requirer"})
	r.AddEndpoint(description.EndpointArgs{ApplicationName: "second", Name: "db", Role: "provider"})

	arg, err := (&importOperation{}).createRemoteImportArg(r, offerer, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(arg.UUID, tc.IsNonZeroUUID)

	// The relation key is still re-written onto the primary application name.
	expectedKey, err := corerelation.NewKeyFromString("client:db first:db")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(arg.Key, tc.DeepEquals, expectedKey)
}

// TestImportLegacyAliasRemapsOnlyAliasUnitSettings ensures that only the unit
// settings belonging to the renamed alias application are re-keyed, and that
// the settings of the other endpoints are passed through unchanged.
func (s *importSuite) TestImportLegacyAliasRemapsOnlyAliasUnitSettings(c *tc.C) {
	m := description.NewModel(description.ModelArgs{})
	offerer := addOfferWithAliases(m, "first", "second")
	r := m.AddRelation(description.RelationArgs{Id: 7, Key: "client:db second:db"})
	epClient := r.AddEndpoint(description.EndpointArgs{ApplicationName: "client", Name: "db", Role: "requirer"})
	epClient.SetUnitSettings("client/0", map[string]any{"private-address": "10.1.2.3"})
	epAlias := r.AddEndpoint(description.EndpointArgs{ApplicationName: "second", Name: "db", Role: "provider"})
	epAlias.SetUnitSettings("second/0", map[string]any{"token": "keep-me"})
	epAlias.SetUnitSettings("second/1", map[string]any{"token": "and-me"})
	key, err := corerelation.NewKeyFromString(r.Key())
	c.Assert(err, tc.ErrorIsNil)
	const token = "6049aa01-76c9-462d-8440-964a6e26aac2"
	arg, err := (&importOperation{}).createRemoteImportArg(r, offerer,
		[]relationRemoteEntity{{RelationKey: key, RelationUUID: token}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(arg.Endpoints, tc.HasLen, 2)
	c.Check(arg.Endpoints[0], tc.DeepEquals, relation.ImportEndpoint{
		ApplicationName:     "client",
		EndpointName:        "db",
		ApplicationSettings: map[string]any{},
		UnitSettings:        map[string]map[string]any{"client/0": {"private-address": "10.1.2.3"}},
	})
	c.Check(arg.Endpoints[1], tc.DeepEquals, relation.ImportEndpoint{
		ApplicationName:     "first",
		EndpointName:        "db",
		ApplicationSettings: map[string]any{},
		UnitSettings: map[string]map[string]any{
			"first/0": {"token": "keep-me"},
			"first/1": {"token": "and-me"},
		},
	})
}

// TestImportRemoteAliasesResolveDistinctTokens exercises the full Execute
// wiring against a source model that holds a relation to each alias of the
// same offer. The relation tokens are exported as remote entities with
// realistic relation tag IDs, one of them listing its endpoints in swapped
// order, as 3.6 may export them. All three relations must re-write onto the
// primary application name while each resolving its own token, otherwise the
// import aborts with a "UNIQUE constraint failed: relation.uuid" error. The
// alias relations' unit settings must be re-keyed onto the primary
// application name.
func (s *importSuite) TestImportRemoteAliasesResolveDistinctTokens(c *tc.C) {
	defer s.setupMocks(c).Finish()

	const (
		tokenFirst  = "6049aa01-76c9-462d-8440-964a6e26aac2"
		tokenSecond = "1d5b7a44-c7e5-4f9a-9b21-3d1f0e6c8a55"
		tokenThird  = "3c2f9b7a-1e46-4d8a-9c3f-7a5b8d9e0f12"
	)

	m := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	addOfferWithAliases(m, "first", "second", "third")

	relPrimary := m.AddRelation(description.RelationArgs{Id: 1, Key: "client:db first:db"})
	relPrimary.AddEndpoint(description.EndpointArgs{
		ApplicationName: "client", Name: "db", Role: "requirer", Interface: "dummy-token",
	})
	epPrimary := relPrimary.AddEndpoint(description.EndpointArgs{
		ApplicationName: "first", Name: "db", Role: "provider", Interface: "dummy-token",
	})
	epPrimary.SetUnitSettings("first/0", map[string]any{"token": "from-primary"})

	relSecond := m.AddRelation(description.RelationArgs{Id: 2, Key: "client:db second:db"})
	relSecond.AddEndpoint(description.EndpointArgs{
		ApplicationName: "client", Name: "db", Role: "requirer", Interface: "dummy-token",
	})
	epSecond := relSecond.AddEndpoint(description.EndpointArgs{
		ApplicationName: "second", Name: "db", Role: "provider", Interface: "dummy-token",
	})
	epSecond.SetUnitSettings("second/0", map[string]any{"token": "keep-me"})

	relThird := m.AddRelation(description.RelationArgs{Id: 3, Key: "client:db third:db"})
	relThird.AddEndpoint(description.EndpointArgs{
		ApplicationName: "client", Name: "db", Role: "requirer", Interface: "dummy-token",
	})
	relThird.AddEndpoint(description.EndpointArgs{
		ApplicationName: "third", Name: "db", Role: "provider", Interface: "dummy-token",
	})

	// One remote entity per relation, as exported by the source model. The
	// second entity lists its endpoints in swapped order.
	m.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-client.db#first.db",
		Token: tokenFirst,
	})
	m.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-second.db#client.db",
		Token: tokenSecond,
	})
	m.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-client.db#third.db",
		Token: tokenThird,
	})

	var args relation.ImportRelationsArgs
	s.service.EXPECT().ImportRelations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, obtained relation.ImportRelationsArgs) error {
			args = obtained
			return nil
		},
	)

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}
	err := importOp.Execute(c.Context(), m)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(args, tc.HasLen, 3)
	c.Check(args[0].ID, tc.Equals, 1)
	c.Check(args[1].ID, tc.Equals, 2)
	c.Check(args[2].ID, tc.Equals, 3)

	// All relations are re-written onto the primary application name, each
	// resolving its own token.
	expectedKey, err := corerelation.NewKeyFromString("client:db first:db")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(args[0].UUID.String(), tc.Equals, tokenFirst)
	c.Check(args[1].UUID.String(), tc.Equals, tokenSecond)
	c.Check(args[2].UUID.String(), tc.Equals, tokenThird)
	for i, arg := range args {
		c.Check(arg.Key, tc.DeepEquals, expectedKey, tc.Commentf("relation %d", i))
	}

	// The alias relations' unit settings follow the endpoint rename; the
	// primary relation's settings pass through unchanged.
	c.Assert(args[0].Endpoints, tc.HasLen, 2)
	c.Check(args[0].Endpoints[1].UnitSettings, tc.DeepEquals,
		map[string]map[string]any{"first/0": {"token": "from-primary"}})
	c.Assert(args[1].Endpoints, tc.HasLen, 2)
	c.Check(args[1].Endpoints[1].UnitSettings, tc.DeepEquals,
		map[string]map[string]any{"first/0": {"token": "keep-me"}})
	c.Assert(args[2].Endpoints, tc.HasLen, 2)
	c.Check(args[2].Endpoints[1].UnitSettings, tc.DeepEquals, map[string]map[string]any{})
}

// TestImportLegacyAliasInvalidRemoteEntityToken documents that the relation
// token from a remote entity is passed through unvalidated at this layer;
// validating it happens later, when the service imports the relation.
func (s *importSuite) TestImportLegacyAliasInvalidRemoteEntityToken(c *tc.C) {
	m := description.NewModel(description.ModelArgs{})
	offerer := addOfferWithAliases(m, "first", "second")
	r := m.AddRelation(description.RelationArgs{Id: 5, Key: "client:db second:db"})
	r.AddEndpoint(description.EndpointArgs{ApplicationName: "client", Name: "db", Role: "requirer"})
	r.AddEndpoint(description.EndpointArgs{ApplicationName: "second", Name: "db", Role: "provider"})
	key, err := corerelation.NewKeyFromString(r.Key())
	c.Assert(err, tc.ErrorIsNil)
	arg, err := (&importOperation{}).createRemoteImportArg(r, offerer,
		[]relationRemoteEntity{{RelationKey: key, RelationUUID: "not-a-uuid"}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(arg.UUID.String(), tc.Equals, "not-a-uuid")
}

// TestRenameUnitSettings checks the re-keying contract: only unit names whose
// application segment matches the renamed alias are re-keyed. Keys without a
// unit-number segment and keys belonging to a different application pass
// through unchanged, so a prefix match cannot rename another application's
// units.
func (s *importSuite) TestRenameUnitSettings(c *tc.C) {
	c.Check(renameUnitSettings(map[string]map[string]any{
		"second/0": {"token": "keep-me"},
		"second/1": {"token": "and-me"},
	}, "second", "first"), tc.DeepEquals, map[string]map[string]any{
		"first/0": {"token": "keep-me"},
		"first/1": {"token": "and-me"},
	})

	c.Check(renameUnitSettings(map[string]map[string]any{
		"secondaire/0": {"token": "other-app"},
		"second":       {"token": "no-unit-number"},
	}, "second", "first"), tc.DeepEquals, map[string]map[string]any{
		"secondaire/0": {"token": "other-app"},
		"second":       {"token": "no-unit-number"},
	})

	c.Check(renameUnitSettings(nil, "second", "first"), tc.DeepEquals,
		map[string]map[string]any{})
}
