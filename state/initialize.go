// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/names/v6"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/storage"
)

// InitializeParams contains the parameters for initializing the state database.
type InitializeParams struct {
	// Clock wraps all calls time. Real uses use clock.WallClock,
	// tests may override with a testing clock.
	Clock clock.Clock

	// ControllerModelArgs contains the arguments for creating
	// the controller model.
	ControllerModelArgs ModelArgs

	// StoragePools is one or more named storage pools to create
	// in the controller model.
	StoragePools map[string]storage.Attrs

	// CloudName contains the name of the cloud that the
	// controller runs in.
	CloudName string

	// ControllerConfig contains config attributes for
	// the controller.
	ControllerConfig controller.Config

	// ControllerInheritedConfig contains default config attributes for
	// models on the specified cloud.
	ControllerInheritedConfig map[string]interface{}

	// RegionInheritedConfig contains region specific configuration for
	// models running on specific cloud regions.
	RegionInheritedConfig cloud.RegionConfig

	// NewPolicy is a function that returns the set of state policies
	// to apply.
	NewPolicy NewPolicyFunc

	// MaxTxnAttempts is the number of attempts when running transactions
	// against mongo. OpenStatePool defaults this if 0.
	MaxTxnAttempts int

	// WatcherPollInterval is the duration of TxnWatcher long-polls. TxnWatcher
	// defaults this if 0.
	WatcherPollInterval time.Duration

	// Note(nvinuesa): Having a dqlite domain service here is an awful hack
	// and should disapear as soon as we migrate units and applications.
	CharmServiceGetter func(modelUUID coremodel.UUID) (CharmService, error)

	// SSHServerHostKey holds the embedded SSH server host key.
	SSHServerHostKey string
}

// Validate checks that the state initialization parameters are valid.
func (p InitializeParams) Validate() error {
	return nil
}

// InitDatabaseFunc defines a function used to
// create the collections and indices in a Juju database.
type InitDatabaseFunc func(string, *controller.Config) error

// Initialize sets up the database with all the collections and indices it needs.
// It also creates the initial model for the controller.
// This needs to be performed only once for the initial controller model.
// It returns unauthorizedError if access is unauthorized.
func Initialize(args InitializeParams) (_ *Controller, err error) {
	controllerTag := names.NewControllerTag(args.ControllerConfig.ControllerUUID())

	modelUUID := args.ControllerModelArgs.UUID.String()
	modelTag := names.NewModelTag(modelUUID)

	ctlr, _ := OpenController(OpenParams{
		Clock:               args.Clock,
		ControllerTag:       controllerTag,
		ControllerModelTag:  modelTag,
		MaxTxnAttempts:      args.MaxTxnAttempts,
		WatcherPollInterval: args.WatcherPollInterval,
		NewPolicy:           args.NewPolicy,
		CharmServiceGetter:  args.CharmServiceGetter,
	})
	return ctlr, nil
}
