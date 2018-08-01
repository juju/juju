// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package txn provides a Runner, which applies operations as part
// of a transaction onto any number of collections within a database.
// The execution of the operations is delegated to a mgo/txn/Runner.
// The purpose of the Runner is to execute the operations multiple
// times in there is a TxnAborted error, in the expectation that subsequent
// attempts will be successful.
// Also included is a mechanism whereby tests can use SetTestHooks to induce
// arbitrary state mutations before and after particular transactions.

package txn

import (
	stderrors "errors"
	"strings"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var logger = loggo.GetLogger("juju.txn")

const (
	// nrRetries is the number of time a transaction will be retried
	// when there is an invariant assertion failure.
	nrRetries = 3

	// defaultTxnCollectionName is the default name of the collection used
	// to initialise the underlying mgo transaction runner.
	defaultTxnCollectionName = "txns"

	// defaultChangeLogName is the default mgo transaction runner change log.
	defaultChangeLogName = "txns.log"
)

var (
	// ErrExcessiveContention is used to signal that even after retrying, the transaction operations
	// could not be successfully applied due to database contention.
	ErrExcessiveContention = stderrors.New("state changing too quickly; try again soon")

	// ErrNoOperations is returned by TransactionSource implementations to signal that
	// no transaction operations are available to run.
	ErrNoOperations = stderrors.New("no transaction operations are available")

	// ErrNoOperations is returned by TransactionSource implementations to signal that
	// the transaction list could not be built but the caller should retry.
	ErrTransientFailure = stderrors.New("transient failure")
)

// TransactionSource defines a function that can return transaction operations to run.
type TransactionSource func(attempt int) ([]txn.Op, error)

// PruneOptions controls when we will trigger a database prune.
type PruneOptions struct {

	// PruneFactor will trigger a prune when the current count of
	// transactions in the database is greater than old*PruneFactor
	PruneFactor float32

	// MinNewTransactions will skip a prune even if pruneFactor is true
	// if there are less than MinNewTransactions that might be cleaned up.
	MinNewTransactions int

	// MaxNewTransactions will force a prune if it sees more than
	// MaxNewTransactions since the last run.
	MaxNewTransactions int

	// MaxTime sets a threshold for 'completed' transactions. Transactions
	// will be considered completed only if they are both older than
	// MaxTime and have a status of Completed or Aborted. Passing the
	// zero Time will cause us to only filter on the Status field.
	MaxTime time.Time
}

// Runner instances applies operations to collections in a database.
type Runner interface {
	// RunTransaction applies the specified transaction operations to a database.
	RunTransaction(ops []txn.Op) error

	// Run calls the nominated function to get the transaction operations to apply to a database.
	// If there is a failure due to a txn.ErrAborted error, the attempt is retried up to nrRetries times.
	Run(transactions TransactionSource) error

	// ResumeTransactions resumes all pending transactions.
	ResumeTransactions() error

	// MaybePruneTransactions removes data for completed transactions
	// from mgo/txn's transaction collection. It is intended to be
	// called periodically.
	//
	// Pruning is an I/O heavy activity so it will only be undertaken
	// if:
	//
	//   txn_count >= pruneFactor * txn_count_at_last_prune
	//
	MaybePruneTransactions(pruneOpts PruneOptions) error
}

type txnRunner interface {
	Run([]txn.Op, bson.ObjectId, interface{}) error
	ResumeAll() error
}

type transactionRunner struct {
	db                        *mgo.Database
	transactionCollectionName string
	changeLogName             string
	testHooks                 chan ([]TestHook)
	runTransactionObserver    func(ObservedTransaction)
	clock                     clock.Clock

	newRunner func() txnRunner
}

var _ Runner = (*transactionRunner)(nil)

// ObservedTransaction is a struct that is passed to RunTransactionObserver whenever a
// transaction is run.
type ObservedTransaction struct {
	// Ops is the operations that were performed
	Ops []txn.Op
	// Error is the error returned from running the operation, might be nil
	Error error
	// Duration is length of time it took to run the operation
	Duration time.Duration
}

// RunnerParams are used to construct a new transaction runner.
// Only the Database value is mandatory, defaults will be used for
// the other attributes if not specified.
type RunnerParams struct {
	// Database is the mgo database for which the transaction runner will be used.
	Database *mgo.Database

	// TransactionCollectionName is the name of the collection
	// used to initialise the underlying mgo transaction runner,
	// defaults to "txns" if unspecified.
	TransactionCollectionName string

	// ChangeLogName is the mgo transaction runner change log,
	// defaults to "txns.log" if unspecified.
	ChangeLogName string

	// RunTransactionObserver, if non-nil, will be called when
	// a Run or RunTransaction call has completed. It will be
	// passed the txn.Ops and the error result.
	RunTransactionObserver func(ObservedTransaction)

	// Clock is an optional clock to use. If Clock is nil, clock.WallClock will
	// be used.
	Clock clock.Clock
}

// NewRunner returns a Runner which runs transactions for the database specified in params.
// Collection names used to manage the transactions and change log may also be specified in
// params, but if not, default values will be used.
func NewRunner(params RunnerParams) Runner {
	txnRunner := &transactionRunner{
		db: params.Database,
		transactionCollectionName: params.TransactionCollectionName,
		changeLogName:             params.ChangeLogName,
		runTransactionObserver:    params.RunTransactionObserver,
		clock:					   params.Clock,
	}
	if txnRunner.transactionCollectionName == "" {
		txnRunner.transactionCollectionName = defaultTxnCollectionName
	}
	if txnRunner.changeLogName == "" {
		txnRunner.changeLogName = defaultChangeLogName
	}
	txnRunner.testHooks = make(chan ([]TestHook), 1)
	txnRunner.testHooks <- nil
	txnRunner.newRunner = txnRunner.newRunnerImpl
	if txnRunner.clock == nil {
		// We allow callers to pass in a nil clock because it is only used if
		// they also specify a RunTransactionObserver.
		txnRunner.clock = clock.WallClock
	}
	return txnRunner
}

func (tr *transactionRunner) newRunnerImpl() txnRunner {
	db := tr.db
	runner := txn.NewRunner(db.C(tr.transactionCollectionName))
	runner.ChangeLog(db.C(tr.changeLogName))
	return runner
}

// Run is defined on Runner.
func (tr *transactionRunner) Run(transactions TransactionSource) error {
	for i := 0; i < nrRetries; i++ {
		ops, err := transactions(i)
		if err == ErrTransientFailure {
			continue
		}
		if err == ErrNoOperations {
			return nil
		}
		if err != nil {
			return err
		}
		if len(ops) == 0 {
			// Treat this the same as ErrNoOperations but don't suppress other errors.
			return nil
		}
		if err := tr.RunTransaction(ops); err == nil {
			return nil
		} else if err != txn.ErrAborted {
			// Mongo very occasionally returns an intermittent
			// "unexpected message" error. Retry those.
			// However if this is the last time, return that error
			// rather than the excessive contention error.
			if !strings.HasSuffix(err.Error(), "unexpected message") || i == (nrRetries-1) {
				return err
			}
		}
	}
	return ErrExcessiveContention
}

// RunTransaction is defined on Runner.
func (tr *transactionRunner) RunTransaction(ops []txn.Op) error {
	testHooks := <-tr.testHooks
	tr.testHooks <- nil
	if len(testHooks) > 0 {
		// Note that this code should only ever be triggered
		// during tests. If we see the log messages below
		// in a production run, something is wrong.
		defer func() {
			if testHooks[0].After != nil {
				logger.Infof("transaction 'after' hook start")
				testHooks[0].After()
				logger.Infof("transaction 'after' hook end")
			}
			if <-tr.testHooks != nil {
				panic("concurrent use of transaction hooks")
			}
			tr.testHooks <- testHooks[1:]
		}()
		if testHooks[0].Before != nil {
			logger.Infof("transaction 'before' hook start")
			testHooks[0].Before()
			logger.Infof("transaction 'before' hook end")
		}
	}
	start := tr.clock.Now()
	runner := tr.newRunner()
	err := runner.Run(ops, "", nil)
	delta := tr.clock.Now().Sub(start)
	if tr.runTransactionObserver != nil {
		tr.runTransactionObserver(ObservedTransaction{
			Ops:      ops,
			Error:    err,
			Duration: delta,
		})
	}
	return err
}

// ResumeTransactions is defined on Runner.
func (tr *transactionRunner) ResumeTransactions() error {
	runner := tr.newRunner()
	return runner.ResumeAll()
}

// MaybePruneTransactions is defined on Runner.
func (tr *transactionRunner) MaybePruneTransactions(pruneOpts PruneOptions) error {
	return maybePrune(tr.db, tr.transactionCollectionName, pruneOpts)
}

// TestHook holds a pair of functions to be called before and after a
// mgo/txn transaction is run.
// Exported only for testing.
type TestHook struct {
	Before func()
	After  func()
}

// TestHooks returns the test hooks for a transaction runner.
// Exported only for testing.
func TestHooks(runner Runner) chan ([]TestHook) {
	return runner.(*transactionRunner).testHooks
}
