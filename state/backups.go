// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/filestorage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

/*
Backups are not a part of juju state nor of normal state operations.
However, they certainly are tightly coupled with state (the very
subject of backups).  This puts backups in an odd position,
particularly with regard to the storage of backup metadata and
archives.  As a result, here are a couple concerns worth mentioning.

First, as noted above backup is about state but not a part of state.
So exposing backup-related methods on State would imply the wrong
thing.  Thus the backup functionality here in the state package (not
state/backups) is exposed as functions to which you pass a state
object.

Second, backup creates an archive file containing a dump of state's
mongo DB.  Storing backup metadata/archives in mongo is thus a
somewhat circular proposition that has the potential to cause
problems.  That may need further attention.

Note that state (and juju as a whole) currently does not have a
persistence layer abstraction to facilitate separating different
persistence needs and implementations.  As a consequence, state's
data, whether about how an environment should look or about existing
resources within an environment, is dumped essentially straight into
State's mongo connection.  The code in the state package does not
make any distinction between the two (nor does the package clearly
distinguish between state-related abstractions and state-related
data).

Backup adds yet another category, merely taking advantage of
State's DB.  In the interest of making the distinction clear, the
code that directly interacts with State (and its DB) lives in this
file.  As mentioned previously, the functionality here is exposed
through functions that take State, rather than as methods on State.
Furthermore, the bulk of the backup-related code, which does not need
direct interaction with State, lives in the state/backups package.
*/

func (st *State) BackupStorage() (filestorage.FileStorage, error) {

}

type DBStorage interface {
	jujutxn.Runner
	io.Closer
	TxnOp(id string) txn.Op
	GetDoc(id string, doc interface{}) error
	ListDocs([]interface{}) error
	AddDoc(id string, doc interface{}) error
}

type mgoStorage struct {
	jujutxn.Runner
	session *mgo.Session
	coll    *mgo.Collection
}

func NewDBStorage(st *State, collName string) DBStorage {
	session := st.db.Session.Copy()
	stor := mgoDocStorage{
		Runner:  st.txnRunner(session),
		session: session,
		coll:    st.db.With(session).C(collName),
	}
	return &stor
}

func (s *mgoStorage) TxnOp(id string) txn.Op {
	return txn.Op{
		C:  s.coll.Name,
		Id: id,
	}
}

func (s *mgoStorage) GetDoc(id string, doc interface{}) error {
	err := s.coll.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("doc %q", id)
	} else if err != nil {
		return errors.Annotatef(err, "while getting doc %q", id)
	}
	return nil
}

func (s *mgoStorage) ListDocs([]interface{}) error {
	if err := s.coll.Find(nil).All(&docs); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (s *mgoStorage) AddDoc(id string, doc interface{}) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		op := s.TxnOp(id)
		op.Assert = txn.DocMissing
		op.Insert = doc
		return []txn.Op{op}, nil
	}
	if err := s.txnRunner.Run(buildTxn); err != nil {
		if err == txn.ErrAborted {
			return errors.AlreadyExistsf("doc %q", id)
		}
		return errors.Annotate(err, "error running transaction")
	}
	return nil
}

func (s *mgoStorage) Close() error {
	s.session.Close()
	return nil
}
