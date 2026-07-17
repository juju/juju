// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbrepl

import (
	"io"
	"maps"

	"github.com/juju/clock"
	"github.com/juju/worker/v5/dependency"

	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/worker/dbrepl"
	"github.com/juju/juju/internal/worker/dbreplaccessor"
	"github.com/juju/juju/internal/worker/terminationworker"
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {
	// NewDBReplWorkerFunc returns a tracked db worker.
	NewDBReplWorkerFunc dbreplaccessor.NewDBReplWorkerFunc

	// DataDir is the controller agent data directory.
	DataDir string

	// CACert is the controller CA certificate.
	CACert string

	// ControllerCert is the controller API certificate.
	ControllerCert string

	// ControllerPrivateKey is the controller API private key.
	ControllerPrivateKey string

	// Clock supplies timekeeping services to various workers.
	Clock clock.Clock

	// Stdout is the writer to use for stdout.
	Stdout io.Writer

	// Stderr is the writer to use for stderr.
	Stderr io.Writer

	// Stdin is the reader to use for stdin.
	Stdin io.Reader
}

// commonManifolds returns manifolds shared between IAAS and CAAS
// controller REPL engines.  The controller binary is always a
// controller, so no ifController gating is required.
func commonManifolds(config ManifoldsConfig) dependency.Manifolds {
	return dependency.Manifolds{
		// The termination worker returns ErrTerminateAgent if a
		// termination signal is received by the process it's running in.
		terminationName: terminationworker.Manifold(),

		// The db-repl manifold drives the interactive REPL worker.
		dbReplName: dbrepl.Manifold(dbrepl.ManifoldConfig{
			DBReplAccessorName: dbReplAccessorName,
			Logger:             internallogger.GetLogger("juju.worker.dbrepl"),
			Stdout:             config.Stdout,
			Stderr:             config.Stderr,
			Stdin:              config.Stdin,
		}),
	}
}

// IAASManifolds returns manifolds for an IAAS controller REPL engine.
func IAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return mergeManifolds(config, dependency.Manifolds{
		dbReplAccessorName: dbreplaccessor.Manifold(dbreplaccessor.ManifoldConfig{
			DataDir:              config.DataDir,
			CACert:               config.CACert,
			ControllerCert:       config.ControllerCert,
			ControllerPrivateKey: config.ControllerPrivateKey,
			Clock:                config.Clock,
			Logger:               internallogger.GetLogger("juju.worker.dbreplaccessor"),
			NewApp:               dbreplaccessor.NewApp,
			NewDBReplWorker:      config.NewDBReplWorkerFunc,
			NewNodeManager:       dbreplaccessor.IAASNodeManager,
		}),
	})
}

// CAASManifolds returns manifolds for a CAAS controller REPL engine.
func CAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return mergeManifolds(config, dependency.Manifolds{
		dbReplAccessorName: dbreplaccessor.Manifold(dbreplaccessor.ManifoldConfig{
			DataDir:              config.DataDir,
			CACert:               config.CACert,
			ControllerCert:       config.ControllerCert,
			ControllerPrivateKey: config.ControllerPrivateKey,
			Clock:                config.Clock,
			Logger:               internallogger.GetLogger("juju.worker.dbreplaccessor"),
			NewApp:               dbreplaccessor.NewApp,
			NewDBReplWorker:      config.NewDBReplWorkerFunc,
			NewNodeManager:       dbreplaccessor.CAASNodeManager,
		}),
	})
}

func mergeManifolds(
	config ManifoldsConfig, manifolds dependency.Manifolds,
) dependency.Manifolds {
	result := commonManifolds(config)
	maps.Copy(result, manifolds)
	return result
}

const (
	dbReplAccessorName = "db-repl-accessor"
	dbReplName         = "db-repl"
	terminationName    = "termination-signal-handler"
)
