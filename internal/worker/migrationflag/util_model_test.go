//go:build dqlite

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	dt "github.com/juju/worker/v5/dependency/testing"

	modelmigrationservice "github.com/juju/juju/domain/modelmigration/service"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/migrationflag"
)

// stubDomainServices is a minimal stub that satisfies services.DomainServices
// for tests that only need ModelMigration().
type stubDomainServices struct {
	services.ControllerDomainServices
	services.ModelDomainServices
}

func (s *stubDomainServices) ModelMigration() *modelmigrationservice.Service {
	return nil
}

// validModelManifoldConfig returns a minimal ModelManifoldConfig
// stuffed with dummy objects that will explode when used.
func validModelManifoldConfig() migrationflag.ModelManifoldConfig {
	return migrationflag.ModelManifoldConfig{
		DomainServicesName: "domain-services",
		ModelUUID:          validUUID,
		Check:              panicCheck,
		NewWorker:          panicWorker,
	}
}

// checkModelManifoldNotValid checks that the supplied
// ModelManifoldConfig creates a manifold that cannot be started.
func checkModelManifoldNotValid(c *tc.C, config migrationflag.ModelManifoldConfig, expect string) {
	manifold := migrationflag.ModelManifold(config)
	worker, err := manifold.Start(c.Context(), dt.StubGetter(nil))
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}
