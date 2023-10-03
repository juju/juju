package dbaccessor

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
)

// DBAppAcquirer is an interface that can be used to reacquire a DBApp.
type DBAppAcquirer interface {
	DBApp() DBApp
	Acquire() error
}

type dbOpener struct {
	acquirer DBAppAcquirer
	mutex    sync.Mutex
	ref      DBApp
	clock    clock.Clock
}

// Open the dqlite database with the given name.
func (a *dbOpener) Open(ctx context.Context, name string) (*sql.DB, error) {
	var db *sql.DB
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			a.mutex.Lock()
			ref := a.ref
			a.mutex.Unlock()

			var err error
			db, err = ref.Open(ctx, name)
			if err == nil {
				return nil
			}

			if err := a.ensureRef(ctx); err != nil {
				return errors.Annotatef(err, "failed to open %q", name)
			}

			return errors.Annotatef(err, "failed to open %q", name)
		},
		Clock:    a.clock,
		Attempts: 3,
		Delay:    time.Second * 10,
	})
	return db, err
}

func (a *dbOpener) ensureRef(ctx context.Context) error {
	if err := a.acquirer.Acquire(); err != nil {
		return errors.Trace(err)
	}

	a.mutex.Lock()
	a.ref = a.acquirer.DBApp()
	a.mutex.Unlock()
	return nil
}
