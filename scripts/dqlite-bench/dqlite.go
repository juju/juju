//go:build dqlite && linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/canonical/go-dqlite/v3/app"
)

type DQLiteDBModelProvider struct {
	a *app.App
}

type shard struct {
	dbs int
	app *app.App
}

type DQLiteDBModelShardProvider struct {
	dbPerShard int
	shards     []*shard
	startPort  int
}

type DQLiteTableModelProvider struct {
	a  *app.App
	db *sql.DB
}

func NewDQLiteDBModelProvider() ModelProvider {
	return &DQLiteDBModelProvider{}
}

func NewDQLiteDBModelShardProvider(dbPerShard int) func() ModelProvider {
	return func() ModelProvider {
		return &DQLiteDBModelShardProvider{
			dbPerShard: dbPerShard,
			startPort:  3555,
		}
	}
}

func NewDQLiteTableModelProvider() ModelProvider {
	return &DQLiteTableModelProvider{}
}

func (d *DQLiteDBModelShardProvider) getShard() (*shard, error) {
	if len(d.shards) == 0 || d.shards[len(d.shards)-1].dbs == d.dbPerShard {
		appDir, err := os.MkdirTemp("", "")
		if err != nil {
			return nil, fmt.Errorf("making temp dir for dqlite: %w", err)
		}
		app, err := app.New(appDir, app.WithAddress(fmt.Sprintf("127.0.0.1:%d", d.startPort+len(d.shards))))
		if err != nil {
			return nil, fmt.Errorf("making dqlite app: %w", err)
		}
		if err := app.Ready(context.Background()); err != nil {
			return nil, fmt.Errorf("making dqlite app ready: %w", err)
		}

		d.shards = append(d.shards, &shard{
			dbs: 0,
			app: app,
		})
	}
	return d.shards[len(d.shards)-1], nil
}

func (d *DQLiteDBModelProvider) Init() error {
	appDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("making temp dir for dqlite: %w", err)
	}

	// app, err := app.New(appDir, app.WithSnapshotParams(dqlite.SnapshotParams{
	//	Trailing:  8192,
	//	Threshold: 8192,
	// }))
	app, err := app.New(appDir)
	if err != nil {
		return fmt.Errorf("making dqlite app: %w", err)
	}
	if err := app.Ready(context.Background()); err != nil {
		return fmt.Errorf("making dqlite app ready: %w", err)
	}

	d.a = app
	return nil
}

func (d *DQLiteDBModelShardProvider) Init() error {
	_, err := d.getShard()
	return err
}

func (d *DQLiteTableModelProvider) Init() error {
	appDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("making temp dir for dqlite: %w", err)
	}

	// app, err := app.New(appDir, app.WithSnapshotParams(dqlite.SnapshotParams{
	//	Trailing:  8192,
	//	Threshold: 8192,
	// }))
	app, err := app.New(appDir)
	if err != nil {
		return fmt.Errorf("making dqlite app: %w", err)
	}
	if err := app.Ready(context.Background()); err != nil {
		return fmt.Errorf("making dqlite app ready: %w", err)
	}

	db, err := app.Open(context.Background(), "db")
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(schema); err != nil {
		_ = tx.Rollback()
		return err
	}

	d.a = app
	d.db = db
	return tx.Commit()
}

func (d *DQLiteDBModelProvider) NewModel(name string) (Model, error) {
	db, err := d.a.Open(context.Background(), name)
	if err != nil {
		return Model{}, err
	}

	tx, err := db.Begin()
	if err != nil {
		return Model{}, err
	}

	if _, err := tx.Exec(schema); err != nil {
		_ = tx.Rollback()
		return Model{}, err
	}

	return Model{
		DB:                  db,
		Name:                name,
		ModelTableName:      "agent",
		ModelEventTableName: "agent_events",
		TxRunner:            transactionRunner(db),
	}, tx.Commit()
}

func (d *DQLiteDBModelShardProvider) NewModel(name string) (Model, error) {
	shard, err := d.getShard()
	if err != nil {
		return Model{}, err
	}

	db, err := shard.app.Open(context.Background(), name)
	if err != nil {
		return Model{}, err
	}
	shard.dbs++

	tx, err := db.Begin()
	if err != nil {
		return Model{}, err
	}
	if _, err := tx.Exec(schema); err != nil {
		_ = tx.Rollback()
		return Model{}, err
	}

	return Model{
		DB:                  db,
		Name:                name,
		ModelTableName:      "agent",
		ModelEventTableName: "agent_events",
		TxRunner:            transactionRunner(db),
	}, tx.Commit()
}

func (d *DQLiteTableModelProvider) NewModel(name string) (Model, error) {
	return Model{
		DB:                  d.db,
		Name:                name,
		ModelTableName:      "agent",
		ModelEventTableName: "agent_events",
		TxRunner:            transactionRunner(d.db),
	}, nil
}
