// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/txn"
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

	// MongoSession is the mgo.Session to use for storing and
	// accessing state data. The caller remains responsible
	// for closing this session; Initialize will copy it.
	MongoSession *mgo.Session

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
	if p.Clock == nil {
		return errors.NotValidf("missing clock")
	}
	if err := p.ControllerModelArgs.Validate(); err != nil {
		return errors.Trace(err)
	}
	if p.MongoSession == nil {
		return errors.NotValidf("nil MongoSession")
	}
	if p.SSHServerHostKey == "" {
		return errors.NotValidf("empty SSHServerHostKey")
	}
	return nil
}

// InitDatabaseFunc defines a function used to
// create the collections and indices in a Juju database.
type InitDatabaseFunc func(*mgo.Session, string, *controller.Config) error

// Initialize sets up the database with all the collections and indices it needs.
// It also creates the initial model for the controller.
// This needs to be performed only once for the initial controller model.
// It returns unauthorizedError if access is unauthorized.
func Initialize(args InitializeParams) (_ *Controller, err error) {
	if err := args.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating initialization args")
	}

	controllerTag := names.NewControllerTag(args.ControllerConfig.ControllerUUID())

	modelUUID := args.ControllerModelArgs.UUID.String()
	modelTag := names.NewModelTag(modelUUID)

	ctlr, err := OpenController(OpenParams{
		Clock:               args.Clock,
		ControllerTag:       controllerTag,
		ControllerModelTag:  modelTag,
		MongoSession:        args.MongoSession,
		MaxTxnAttempts:      args.MaxTxnAttempts,
		WatcherPollInterval: args.WatcherPollInterval,
		NewPolicy:           args.NewPolicy,
		InitDatabaseFunc:    InitDatabase,
		CharmServiceGetter:  args.CharmServiceGetter,
	})
	if err != nil {
		return nil, errors.Annotate(err, "opening controller")
	}
	defer func() {
		if err != nil {
			if closeErr := ctlr.Close(); closeErr != nil {
				logger.Errorf(context.TODO(), "error closing controller while aborting Initialize: %v", closeErr)
			}
		}
	}()

	// The system state is owned by the pool, which is closed by the
	// controller close, so no close needed here.
	st, err := ctlr.pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// A valid model is used as a signal that the
	// state has already been initialized. If this is the case
	// do nothing.
	if _, err := st.Model(); err == nil {
		return nil, errors.New("already initialized")
	} else if !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}

	logger.Infof(context.TODO(), "initializing controller model %s", modelTag.Id())

	modelOps, err := st.modelSetupOps(
		args.ControllerConfig.ControllerUUID(),
		args.ControllerModelArgs,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var ops []txn.Op
	ops = append(ops,
		txn.Op{
			C:  controllersC,
			Id: sshServerHostKeyDocId,
			Insert: &sshServerHostKeyDoc{
				Key: args.SSHServerHostKey,
			},
		},
		txn.Op{
			C:      controllersC,
			Id:     modelGlobalKey,
			Assert: txn.DocMissing,
			Insert: &controllersDoc{
				CloudName: args.CloudName,
				ModelUUID: st.ModelUUID(),
			},
		},
		txn.Op{
			C:      controllersC,
			Id:     hostedModelCountKey,
			Assert: txn.DocMissing,
			Insert: &hostedModelCountDoc{},
		},
	)

	ops = append(ops, modelOps...)

	if err := st.db().RunTransaction(ops); err != nil {
		return nil, errors.Trace(err)
	}
	return ctlr, nil
}

// InitDatabase creates all the collections and indices in a Juju database.
func InitDatabase(session *mgo.Session, modelUUID string, settings *controller.Config) error {
	schema := allCollections()
	if err := schema.Create(session.DB(jujuDB), settings); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// modelSetupOps returns the transactions necessary to set up a model.
func (st *State) modelSetupOps(controllerUUID string, args ModelArgs) ([]txn.Op, error) {
	controllerModelUUID := st.controllerModelTag.Id()
	modelUUID := args.UUID.String()

	ops := []txn.Op{}
	// Inc ref count for hosted models.
	if controllerModelUUID != modelUUID {
		ops = append(ops, incHostedModelCountOp())
	}

	ops = append(ops,
		createModelEntityRefsOp(modelUUID),
		createModelOp(
			args.Type,
			args.Owner,
			args.Name,
			modelUUID,
			controllerUUID,
			args.CloudName,
			args.CloudRegion,
			args.PasswordHash,
			args.CloudCredential,
		),
		createUniqueOwnerModelNameOp(args.Owner, args.Name),
	)
	return ops, nil
}
