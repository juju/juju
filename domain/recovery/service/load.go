// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"slices"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	domainexport "github.com/juju/juju/domain/export"
	"github.com/juju/juju/internal/errors"
)

// LoadControllerDump validates native YAML before replacing controller data
// in a single transaction. Replay policy keeps every source lease out of the
// replacement, so ownership held by the lost controller can never block
// backups on its replacement.
func (s *Service) LoadControllerDump(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return errors.Capture(err)
	}
	dump, err := decodeDump(data)
	if err != nil {
		return errors.Capture(err)
	}
	if err := checkDumpVersion(dump, []semversion.Number{domainexport.LatestControllerExportVersion()}); err != nil {
		return errors.Errorf("controller dump: %w", err)
	}
	if len(dump.Tables["controller"]) == 0 {
		return errors.New("controller dump has no controller row")
	}
	decoded, err := decodeControllerDump(dump)
	if err != nil {
		return errors.Capture(err)
	}
	selectTables(decoded.Present, controllerTablePolicies)
	return errors.Capture(s.controller.LoadControllerDump(ctx, decoded, s.nodeID))
}

// LoadModelDump validates native YAML before opening and replacing model
// data. Imported placements are rebound to the replacement's object store
// node supplied at construction, which also serves every model blob.
func (s *Service) LoadModelDump(ctx context.Context, model coremodel.UUID, data []byte) error {
	if err := ctx.Err(); err != nil {
		return errors.Capture(err)
	}
	if err := model.Validate(); err != nil {
		return errors.Capture(err)
	}
	dump, err := decodeDump(data)
	if err != nil {
		return errors.Capture(err)
	}
	if err := checkDumpVersion(dump, []semversion.Number{domainexport.LatestSupportedPayloadVersion()}); err != nil {
		return errors.Errorf("model dump: %w", err)
	}
	decoded, err := decodeModelDump(dump)
	if err != nil {
		return errors.Capture(err)
	}
	selectTables(decoded.Present, modelTablePolicies)
	st, err := s.modelState(ctx, model)
	if err != nil {
		return errors.Capture(err)
	}
	if err := st.LoadModelDump(ctx, decoded, s.nodeID); err != nil {
		return errors.Errorf("model %q: %w", model, err)
	}
	return nil
}

func selectTables(present map[string]bool, policies map[string]tablePolicy) {
	for table := range present {
		if policies[table] == policySkip || specialTables[table] {
			delete(present, table)
		}
	}
}

// checkDumpVersion enforces the loader's native dump formats: a dump is
// only ever loaded when its envelope version names an export format this
// binary produces. The agent-version gate implies matching formats, but
// the envelope is the actual contract and is checked where the load runs.
func checkDumpVersion(dump *dump, supported []semversion.Number) error {
	if dump.Version == "" {
		return errors.Errorf("dump records no format version")
	}
	parsed, err := semversion.Parse(dump.Version)
	if err != nil {
		return errors.Errorf("dump records unparseable format version %q", dump.Version)
	}
	if !slices.Contains(supported, parsed) {
		return errors.Errorf("unsupported dump format version %q: this loader reads %v", dump.Version, supported)
	}
	return nil
}
