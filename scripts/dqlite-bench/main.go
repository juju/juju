// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/database/txn"
)

type ModelProvider interface {
	Init() error
	NewModel(string) (Model, error)
}

type TxRunner func(context.Context, func(context.Context, *sql.Tx) error) error

type Model struct {
	*sql.DB
	Name                string
	ModelTableName      string
	ModelEventTableName string
	TxRunner            TxRunner
}

type ModelOperationDef struct {
	opName string
	op     ModelOperation
	freq   time.Duration
}

const (
	// Control the number of models created in the test and the frequency at
	// which they are added.
	AddModelRate         = 400
	DatabaseAddFrequency = time.Second
	MaxNumberOfDatabases = 400
)

const (
	schema = `
CREATE TABLE agent (
    uuid TEXT PRIMARY KEY,
    model_name TEXT NOT NULL,
    status TEXT NOT NULL
);

CREATE INDEX idx_agent_model_name ON agent (model_name);
CREATE INDEX idx_agent_status ON agent (status);

CREATE TABLE agent_events (
 	agent_uuid TEXT NOT NULL,   
 	event TEXT NOT NULL,
 	CONSTRAINT fk_agent_uuid
    	FOREIGN KEY (agent_uuid)
        REFERENCES agent(uuid)
);

CREATE INDEX idx_agent_events_event ON agent_events (event);
`
)

var (
	_ = schema

	modelCreationTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "model_creation_time",
		Buckets: []float64{
			0.001,
			0.01,
			0.1,
			1.0,
			10.0,
		},
	})

	modelTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "model_total",
		Help: "The total number of models",
	})

	modelAgentGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "model_agents",
	}, []string{"model"})

	modelAgentEventsGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "model_agent_events",
	}, []string{"model"})

	// Valid values are
	// - NewDQLiteDBModelProvider for having a database per model
	// - NewDQLiteDBModelShardProvider for having a database per model sharded
	// over many dqlite applications.
	// - NewDQLiteTableModelProvider for having one db for all models created.
	newModelProvider = NewDQLiteDBModelProvider

	// Valid values are
	// - SlotPerDBBasedTransactionRunner for having a fixed number of slots per
	// database created.
	// - SlotAllDBBasedTransactionRunner for having a fixed number of slots
	// across all databases created.
	// - SingleConnectionTransactionRunner for limiting the number of database
	// connections to 1 in the dqlite driver.
	transactionRunner = SlotAllDBBasedTransactionRunner(runtime.GOMAXPROCS(0))
	_                 = transactionRunner

	// Set the operations to be performed per model and the frequency.
	perModelOperations = []ModelOperationDef{
		{
			opName: "model-init",
			op:     seedModelAgents(60),
			freq:   time.Duration(0),
		},
		{
			opName: "agent-status-active",
			op:     updateModelAgentStatus(10, "active"),
			freq:   time.Second * 5,
		},
		{
			opName: "agent-status-inactive",
			op:     updateModelAgentStatus(10, "inactive"),
			freq:   time.Second * 8,
		},
		{
			opName: "agent-events",
			op:     generateAgentEvents(10),
			freq:   time.Second * 15,
		},
		{
			opName: "cull-agent-events",
			op:     cullAgentEvents(30),
			freq:   time.Second * 30,
		},
		{
			opName: "agents-count",
			op:     agentModelCount(modelAgentGauge),
			freq:   time.Second * 30,
		},
		{
			opName: "agent-events-count",
			op:     agentEventModelCount(modelAgentEventsGauge),
			freq:   time.Second * 30,
		},
	}
)

func SlotPerDBBasedTransactionRunner(slots int) func(*sql.DB) TxRunner {
	return func(db *sql.DB) TxRunner {
		slotCh := make(chan any, slots)
		txnRunner := txn.NewRetryingTxnRunner()

		return TxRunner(func(c context.Context, fn func(context.Context, *sql.Tx) error) error {
			slotCh <- nil
			defer func() { <-slotCh }()
			err := txnRunner.Retry(c, func() error {
				return txnRunner.StdTxn(c, db, fn)
			})
			return err
		})
	}
}

func SlotAllDBBasedTransactionRunner(slots int) func(*sql.DB) TxRunner {
	slotCh := make(chan any, slots)

	return func(db *sql.DB) TxRunner {
		txnRunner := txn.NewRetryingTxnRunner()

		return TxRunner(func(c context.Context, fn func(context.Context, *sql.Tx) error) error {
			slotCh <- nil
			defer func() { <-slotCh }()
			err := txnRunner.Retry(c, func() error {
				return txnRunner.StdTxn(c, db, fn)
			})
			return err
		})
	}
}

func SingleConnectionTransactionRunner(db *sql.DB) TxRunner {
	db.SetMaxOpenConns(1)

	return TxRunner(func(c context.Context, fn func(context.Context, *sql.Tx) error) error {
		tx, err := db.BeginTx(c, nil)
		if err != nil {
			return err
		}

		if err := fn(c, tx); err != nil {
			return errors.Join(err, tx.Rollback())
		}

		return tx.Commit()
	})
}

func RetryableTransactionRunner(db *sql.DB) TxRunner {
	txnRunner := txn.NewRetryingTxnRunner()

	return TxRunner(func(c context.Context, fn func(context.Context, *sql.Tx) error) error {
		return txnRunner.Retry(c, func() error {
			return txnRunner.StdTxn(c, db, fn)
		})
	})
}

func main() {
	var err error
	if _, err = os.Stat("/tmp"); errors.Is(err, fs.ErrNotExist) {
		err = os.Mkdir("/tmp", 0750)
	}
	if err != nil {
		fmt.Printf("establishing tmp dir: %v\n", err)
		os.Exit(1)
	}

	provider := newModelProvider()

	if err := provider.Init(); err != nil {
		fmt.Printf("init model provider: %v\n", err)
		os.Exit(1)
	}

	t := tomb.Tomb{}

	mux := http.NewServeMux()
	server := http.Server{
		Addr:         ":3333",
		Handler:      mux,
		WriteTimeout: 50 * time.Second,
	}
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))

	t.Go(func() error {
		return server.ListenAndServe()
	})

	modelCh := modelRamper(&t, provider, DatabaseAddFrequency, AddModelRate, MaxNumberOfDatabases)
	modelSpawner(&t, modelCh, perModelOperations)

	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-t.Dead():
	case <-sig:
		t.Kill(nil)
		server.Close()
	}

	err = t.Wait()
	fmt.Println(err)
}

func modelSpawner(
	t *tomb.Tomb,
	ch <-chan Model,
	perModelOperations []ModelOperationDef,
) {
	startPerModelOperations := func(opTomb *tomb.Tomb, models []Model) {
		for _, model := range models {
			for _, op := range perModelOperations {
				RunModelOperation(opTomb, op.opName, op.freq, op.op, model)
			}
		}
	}

	t.Go(func() error {
		opTomb := tomb.Tomb{}
		allModels := []Model{}
		models := []Model{}

		for {
			select {
			case model, ok := <-ch:
				if !ok {
					ch = nil
					break
				}
				models = append(models, model)
			case <-t.Dying():
				opTomb.Kill(nil)
				return opTomb.Wait()
			case <-opTomb.Dead():
				err := opTomb.Wait()
				fmt.Printf("operation tomb is dead: %v", err)
				return err
			default:
				if len(models) == 0 {
					break
				}
				allModels = append(allModels, models...)
				models = []Model{}
				opTomb.Kill(nil)
				if opTomb.Alive() {
					if err := opTomb.Wait(); err != nil {
						fmt.Println("Tomb error", err)
						return err
					}
				}
				opTomb = tomb.Tomb{}
				fmt.Printf("Spawning model %d operations\n", AddModelRate)
				startPerModelOperations(&opTomb, allModels)
			}
		}
	})
}

func modelRamper(
	t *tomb.Tomb,
	provider ModelProvider,
	freq time.Duration,
	inc,
	max int,
) <-chan Model {
	newDBCh := make(chan Model, inc)
	t.Go(func() error {
		defer close(newDBCh)
		ticker := time.NewTicker(freq)
		numDBS := 0
		for numDBS < max {
			select {
			case <-t.Dying():
				return nil
			case <-ticker.C:
			}
			dbs, makeErr := makeModels(provider, inc)
			numDBS += len(dbs)
			modelTotal.Add(float64(len(dbs)))

			for _, db := range dbs {
				newDBCh <- db
			}

			if makeErr != nil {
				return makeErr
			}
		}
		return nil
	})
	return newDBCh
}

func makeModels(provider ModelProvider, x int) ([]Model, error) {
	models := make([]Model, 0, x)
	for i := 0; i < x; i++ {
		model, err := func() (Model, error) {
			timer := prometheus.NewTimer(modelCreationTime)
			defer timer.ObserveDuration()
			dbUUID := uuid.New()
			return provider.NewModel(dbUUID.String())
		}()

		if err != nil {
			return models, err
		}
		models = append(models, model)
	}

	return models, nil
}
