// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/mongo/utils"
	"github.com/juju/juju/state/watcher"
)

// CreateSecretBackendParams are used to create a secret backend.
type CreateSecretBackendParams struct {
	ID                  string
	Name                string
	BackendType         string
	TokenRotateInterval *time.Duration
	NextRotateTime      *time.Time
	Config              map[string]interface{}
}

// UpdateSecretBackendParams are used to update a secret backend.
type UpdateSecretBackendParams struct {
	ID                  string
	NameChange          *string
	TokenRotateInterval *time.Duration
	NextRotateTime      *time.Time
	Config              map[string]interface{}
}

// SecretBackendsStorage instances use mongo to store secret backend info.
type SecretBackendsStorage interface {
	CreateSecretBackend(params CreateSecretBackendParams) (string, error)
	UpdateSecretBackend(params UpdateSecretBackendParams) error
	DeleteSecretBackend(name string, force bool) error
	ListSecretBackends() ([]*secrets.SecretBackend, error)
	GetSecretBackend(name string) (*secrets.SecretBackend, error)
	GetSecretBackendByID(ID string) (*secrets.SecretBackend, error)
	SecretBackendRotated(ID string, next time.Time) error
}

// NewSecretBackends creates a new mongo backed secrets storage.
func NewSecretBackends(st *State) *secretBackendsStorage {
	return &secretBackendsStorage{st: st}
}

type secretBackendDoc struct {
	DocID string `bson:"_id"`

	Name                string           `bson:"name"`
	BackendType         string           `bson:"backend-type"`
	TokenRotateInterval *time.Duration   `bson:"token-rotate-interval,omitempty"`
	Config              backendConfigMap `bson:"config,omitempty"`
}

type backendConfigMap map[string]interface{}

func (m *backendConfigMap) SetBSON(raw bson.Raw) error {
	rawMap := make(map[string]interface{})
	if err := raw.Unmarshal(rawMap); err != nil {
		return err
	}
	*m = utils.UnescapeKeys(rawMap)
	return nil
}

func (m backendConfigMap) GetBSON() (interface{}, error) {
	escapedMap := utils.EscapeKeys(m)
	return escapedMap, nil
}

type secretBackendsStorage struct {
	st *State
}

func (s *secretBackendsStorage) secretBackendDoc(p *CreateSecretBackendParams) (*secretBackendDoc, error) {
	if p.TokenRotateInterval != nil && p.NextRotateTime == nil {
		return nil, errors.NotValidf("secret backend missing next rotate time")
	}
	id := p.ID
	if id == "" {
		id = bson.NewObjectId().Hex()
	}
	backend := &secretBackendDoc{
		DocID:               id,
		Name:                p.Name,
		BackendType:         p.BackendType,
		TokenRotateInterval: p.TokenRotateInterval,
		Config:              p.Config,
	}
	return backend, nil
}

// CreateSecretBackend creates a new secret backend.
func (s *secretBackendsStorage) CreateSecretBackend(p CreateSecretBackendParams) (string, error) {
	if p.ID != "" {
		_, err := s.GetSecretBackendByID(p.ID)
		if err == nil {
			return "", errors.AlreadyExistsf("secret backend with id %q", p.ID)
		}
	}
	backendDoc, err := s.secretBackendDoc(&p)
	if err != nil {
		return "", errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// This isn't perfect but we don't want to use the name as the doc id.
		// The tiny window for multiple callers to create dupe backends will
		// go away once we transition to a SQL backend.
		if _, err := s.GetSecretBackend(p.Name); err != nil {
			if !errors.Is(err, errors.NotFound) {
				return nil, errors.Annotatef(err, "checking for existing secret backend")
			}
		} else {
			return nil, errors.AlreadyExistsf("secret backend %q", p.Name)
		}
		ops := []txn.Op{{
			C:      secretBackendsC,
			Id:     backendDoc.DocID,
			Assert: txn.DocMissing,
			Insert: *backendDoc,
		}}
		refCountOps, err := s.st.createSecretBackendRefCountOp(backendDoc.DocID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, refCountOps...)

		if p.NextRotateTime != nil {
			rotateOps, err := s.tokenRotationOps(backendDoc.DocID, &p.Name, p.NextRotateTime)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, rotateOps...)
		}
		return ops, nil
	}
	return backendDoc.DocID, errors.Trace(s.st.db().Run(buildTxn))
}

func (s *secretBackendsStorage) toSecretBackend(doc *secretBackendDoc) *secrets.SecretBackend {
	return &secrets.SecretBackend{
		ID:                  doc.DocID,
		Name:                doc.Name,
		BackendType:         doc.BackendType,
		TokenRotateInterval: doc.TokenRotateInterval,
		Config:              doc.Config,
	}
}

func (st *State) checkBackendExists(ID string) error {
	secretBackendCollection, closer := st.db().GetCollection(secretBackendsC)
	defer closer()
	n, err := secretBackendCollection.FindId(ID).Count()
	if err != nil {
		return errors.Trace(err)
	}
	if n == 0 {
		return errors.NotFoundf("secret backend %q", ID)
	}
	return nil
}

// UpdateSecretBackend updates a new secret backend.
func (s *secretBackendsStorage) UpdateSecretBackend(p UpdateSecretBackendParams) error {
	secretBackendsColl, closer := s.st.db().GetCollection(secretBackendsC)
	defer closer()

	var doc secretBackendDoc
	err := secretBackendsColl.FindId(p.ID).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("secret backend %q", p.ID)
	}
	if err != nil {
		return errors.Trace(err)
	}
	if p.TokenRotateInterval != nil {
		if *p.TokenRotateInterval != 0 {
			doc.TokenRotateInterval = p.TokenRotateInterval
		} else {
			doc.TokenRotateInterval = nil
		}
	}
	if p.NameChange != nil {
		doc.Name = *p.NameChange
	}
	doc.Config = p.Config
	update := bson.D{{"$set", doc}}
	if doc.TokenRotateInterval == nil {
		update = append(update, bson.DocElem{"$unset", bson.D{{"token-rotate-interval", nil}}})
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if p.NameChange != nil {
			inUse, err := s.st.isSecretBackendInUse(p.ID)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if inUse {
				return nil, errors.NewNotValid(nil, "cannot rename a secret backend that is in use")
			}

			// This isn't perfect but we don't want to use the name as the doc id.
			// The tiny window for multiple callers to create dupe backends will
			// go away once we transition to a SQL backend.
			if existing, err := s.GetSecretBackend(doc.Name); err != nil {
				if !errors.Is(err, errors.NotFound) {
					return nil, errors.Annotatef(err, "checking for existing secret backend")
				}
			} else if existing.ID != p.ID {
				return nil, errors.AlreadyExistsf("secret backend %q", doc.Name)
			}
		}

		n, err := secretBackendsColl.FindId(p.ID).Count()
		if n == 0 || err != nil {
			return nil, errors.NotFoundf("secret backend %q", p.ID)
		}
		ops := []txn.Op{{
			C:      secretBackendsC,
			Id:     doc.DocID,
			Assert: txn.DocExists,
			Update: update,
		}}
		if p.NameChange != nil || p.TokenRotateInterval != nil {
			nextRotateTime := p.NextRotateTime
			if doc.TokenRotateInterval == nil {
				nextRotateTime = nil
			}
			nameChange := p.NameChange
			if nameChange == nil && nextRotateTime != nil {
				nameChange = &doc.Name
			}
			rotateOps, err := s.tokenRotationOps(p.ID, nameChange, nextRotateTime)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, rotateOps...)
		}
		return ops, nil
	}
	return errors.Trace(s.st.db().Run(buildTxn))
}

// GetSecretBackend gets the named secret backend.
func (s *secretBackendsStorage) GetSecretBackend(name string) (*secrets.SecretBackend, error) {
	secretBackendsColl, closer := s.st.db().GetCollection(secretBackendsC)
	defer closer()

	var doc secretBackendDoc
	err := secretBackendsColl.Find(bson.D{{"name", name}}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("secret backend %q", name)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.toSecretBackend(&doc), nil
}

// GetSecretBackendByID gets the secret backend with the given ID.
func (s *secretBackendsStorage) GetSecretBackendByID(ID string) (*secrets.SecretBackend, error) {
	secretBackendsColl, closer := s.st.db().GetCollection(secretBackendsC)
	defer closer()

	var doc secretBackendDoc
	err := secretBackendsColl.FindId(ID).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("secret backend %q", ID)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.toSecretBackend(&doc), nil
}

// ListSecretBackends lists the secret backends.
func (s *secretBackendsStorage) ListSecretBackends() ([]*secrets.SecretBackend, error) {
	secretBackendCollection, closer := s.st.db().GetCollection(secretBackendsC)
	defer closer()

	var docs []secretBackendDoc
	err := secretBackendCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]*secrets.SecretBackend, len(docs))
	for i, doc := range docs {
		result[i] = s.toSecretBackend(&doc)
	}
	return result, nil
}

// DeleteSecretBackend deletes the specified secret backend.
func (s *secretBackendsStorage) DeleteSecretBackend(name string, force bool) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		b, err := s.GetSecretBackend(name)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				return nil, jujutxn.ErrNoOperations
			}
			return nil, errors.Trace(err)
		}
		deleteOp := txn.Op{
			C:      secretBackendsC,
			Id:     b.ID,
			Assert: txn.DocExists,
			Remove: true,
		}

		refCountOp, err := s.st.removeBackendRefCountOp(b.ID, force)
		if err != nil {
			return nil, errors.Trace(err)
		}

		return append([]txn.Op{deleteOp}, refCountOp...), nil
	}
	return errors.Trace(s.st.db().Run(buildTxn))
}

func secretBackendRefCountKey(backendID string) string {
	return fmt.Sprintf("secretbackend#revisions#%s", backendID)
}

func (st *State) isSecretBackendInUse(backendID string) (bool, error) {
	refCountCollection, ccloser := st.db().GetCollection(globalRefcountsC)
	defer ccloser()
	_, count, err := nsRefcounts.CurrentOp(refCountCollection, secretBackendRefCountKey(backendID))
	if err != nil {
		return false, errors.Trace(err)
	}
	return count > 0, nil
}

func (st *State) removeBackendRefCountOp(backendID string, force bool) ([]txn.Op, error) {
	if secrets.IsInternalSecretBackendID(backendID) {
		return nil, nil
	}
	if force {
		// If we are forcing removal, simply delete any ref count reference.
		op := nsRefcounts.JustRemoveOp(globalRefcountsC, secretBackendRefCountKey(backendID), -1)
		return []txn.Op{op}, nil
	}

	refCountCollection, ccloser := st.db().GetCollection(globalRefcountsC)
	defer ccloser()

	_, count, err := nsRefcounts.CurrentOp(refCountCollection, secretBackendRefCountKey(backendID))
	if err != nil {
		return nil, errors.Trace(err)
	}
	if count > 0 {
		return nil, errors.NotSupportedf("removing backend with %d stored secret revisions", count)
	}
	op := nsRefcounts.JustRemoveOp(globalRefcountsC, secretBackendRefCountKey(backendID), count)
	return []txn.Op{op}, nil
}

func (st *State) createSecretBackendRefCountOp(backendID string) ([]txn.Op, error) {
	if secrets.IsInternalSecretBackendID(backendID) {
		return nil, nil
	}
	refCountCollection, ccloser := st.db().GetCollection(globalRefcountsC)
	defer ccloser()
	op, err := nsRefcounts.StrictCreateOp(refCountCollection, secretBackendRefCountKey(backendID), 0)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return []txn.Op{op}, nil
}

func (st *State) decSecretBackendRefCountOp(backendID string) ([]txn.Op, error) {
	if secrets.IsInternalSecretBackendID(backendID) {
		return nil, nil
	}
	refCountCollection, ccloser := st.db().GetCollection(globalRefcountsC)
	defer ccloser()

	op, err := nsRefcounts.AliveDecRefOp(refCountCollection, secretBackendRefCountKey(backendID))
	if errors.Is(err, errors.NotFound) || errors.Cause(err) == errRefcountAlreadyZero {
		return nil, nil
	}
	return []txn.Op{op}, errors.Trace(err)
}

// incBackendRevisionCountOps returns the ops needed to change the secret revision ref count
// for the specified backend. Used to ensure backends with revisions cannot be deleted without force.
func (st *State) incBackendRevisionCountOps(backendID string, count int) ([]txn.Op, error) {
	if secrets.IsInternalSecretBackendID(backendID) {
		return nil, nil
	}
	refCountCollection, ccloser := st.db().GetCollection(globalRefcountsC)
	defer ccloser()

	key := secretBackendRefCountKey(backendID)
	countOp, _, err := nsRefcounts.CurrentOp(refCountCollection, key)
	if err != nil {
		return nil, errors.Trace(err)
	}
	incOp, err := nsRefcounts.StrictIncRefOp(refCountCollection, key, count)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return []txn.Op{countOp, incOp}, nil
}

type secretBackendRotationDoc struct {
	DocID    string `bson:"_id"`
	TxnRevno int64  `bson:"txn-revno"`

	// These fields are denormalised here so that the watcher
	// only needs to access this collection.
	Name           string    `bson:"backend-name"`
	NextRotateTime time.Time `bson:"next-rotate-time"`
}

func (s *secretBackendsStorage) tokenRotationOps(ID string, name *string, nextRotateTime *time.Time) ([]txn.Op, error) {
	if nextRotateTime == nil && name == nil {
		return []txn.Op{{
			C:      secretBackendsRotateC,
			Id:     ID,
			Remove: true,
		}}, nil
	}
	secretBackendRotateCollection, closer := s.st.db().GetCollection(secretBackendsRotateC)
	defer closer()

	var doc secretBackendRotationDoc
	err := secretBackendRotateCollection.FindId(ID).One(&doc)
	if err == mgo.ErrNotFound {
		return []txn.Op{{
			C:      secretBackendsRotateC,
			Id:     ID,
			Assert: txn.DocMissing,
			Insert: secretBackendRotationDoc{
				DocID:          ID,
				Name:           *name,
				NextRotateTime: (*nextRotateTime).Round(time.Second).UTC(),
			},
		}}, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	toUpdate := bson.M{}
	if name != nil {
		toUpdate["backend-name"] = *name
	}
	if nextRotateTime != nil {
		toUpdate["next-rotate-time"] = nextRotateTime.Round(time.Second).UTC()
	}

	return []txn.Op{{
		C:      secretBackendsRotateC,
		Id:     ID,
		Assert: txn.DocExists,
		Update: bson.M{"$set": toUpdate},
	}}, nil
}

// SecretBackendRotated records that the given secret backend token was rotated and
// sets the next rotate time.
func (s *secretBackendsStorage) SecretBackendRotated(ID string, next time.Time) error {
	secretBadckendRotateCollection, closer := s.st.db().GetCollection(secretBackendsRotateC)
	defer closer()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if err := s.st.checkBackendExists(ID); err != nil {
			return nil, errors.Trace(err)
		}

		var currentDoc secretBackendRotationDoc
		err := secretBadckendRotateCollection.FindId(ID).One(&currentDoc)
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, errors.NotFoundf("token rotation info for secret backend %q", ID)
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		// If next rotate time is sooner than our proposed time, keep the existing value.
		if attempt > 0 && currentDoc.NextRotateTime.Before(next) {
			return nil, jujutxn.ErrNoOperations
		}
		ops := []txn.Op{{
			C:      secretBackendsRotateC,
			Id:     ID,
			Assert: bson.D{{"txn-revno", currentDoc.TxnRevno}},
			Update: bson.M{"$set": bson.M{
				"next-rotate-time": next,
			}},
		}}
		return ops, nil
	}
	return s.st.db().Run(buildTxn)
}

// WatchSecretBackendRotationChanges returns a watcher for token  rotation updates
// to secret backends.
func (st *State) WatchSecretBackendRotationChanges() (SecretBackendRotateWatcher, error) {
	return newSecretBackendRotationWatcher(st), nil
}

// SecretBackendRotateWatcher defines a watcher for changes to
// secret backend rotation config.
type SecretBackendRotateWatcher interface {
	Watcher
	Changes() corewatcher.SecretBackendRotateChannel
}

type secretBackendRotateWatcherDetails struct {
	txnRevNo int64
	ID       string
	Name     string
}

type secretBackendRotationWatcher struct {
	commonWatcher
	out chan []corewatcher.SecretBackendRotateChange

	known map[string]secretBackendRotateWatcherDetails
}

func newSecretBackendRotationWatcher(backend modelBackend) *secretBackendRotationWatcher {
	w := &secretBackendRotationWatcher{
		commonWatcher: newCommonWatcher(backend),
		out:           make(chan []corewatcher.SecretBackendRotateChange),
		known:         make(map[string]secretBackendRotateWatcherDetails),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

// Changes returns a channel that will receive changes when the next rotate time
// for a secret backend changes.
func (w *secretBackendRotationWatcher) Changes() corewatcher.SecretBackendRotateChannel {
	return w.out
}

func (w *secretBackendRotationWatcher) initial() ([]corewatcher.SecretBackendRotateChange, error) {
	var details []corewatcher.SecretBackendRotateChange

	var doc secretBackendRotationDoc
	secretBackendRotateCollection, closer := w.db.GetCollection(secretBackendsRotateC)
	defer closer()

	iter := secretBackendRotateCollection.Find(nil).Iter()
	for iter.Next(&doc) {
		ID := w.backend.localID(doc.DocID)
		w.known[doc.DocID] = secretBackendRotateWatcherDetails{
			txnRevNo: doc.TxnRevno,
			ID:       ID,
			Name:     doc.Name,
		}
		details = append(details, corewatcher.SecretBackendRotateChange{
			ID:              ID,
			Name:            doc.Name,
			NextTriggerTime: doc.NextRotateTime.UTC(),
		})
	}
	return details, errors.Trace(iter.Close())
}

func (w *secretBackendRotationWatcher) merge(details []corewatcher.SecretBackendRotateChange, change watcher.Change) ([]corewatcher.SecretBackendRotateChange, error) {
	changeID := change.Id.(string)
	knownDetails, known := w.known[changeID]

	doc := secretBackendRotationDoc{}
	if change.Revno >= 0 {
		// Record added or updated.
		secretBackendRotateColl, closer := w.db.GetCollection(secretBackendsRotateC)
		defer closer()
		err := secretBackendRotateColl.FindId(change.Id).One(&doc)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		// Changed but no longer in the collection so ignore.
		if err != nil {
			return details, nil
		}
	} else if known {
		// Record deleted.
		delete(w.known, changeID)
		deletedDetails := corewatcher.SecretBackendRotateChange{
			ID:   knownDetails.ID,
			Name: knownDetails.Name,
		}
		for i, detail := range details {
			if detail.ID == changeID {
				details[i] = deletedDetails
				return details, nil
			}
		}
		details = append(details, deletedDetails)
		return details, nil
	}
	if doc.TxnRevno > knownDetails.txnRevNo {
		ID := w.backend.localID(doc.DocID)
		w.known[changeID] = secretBackendRotateWatcherDetails{
			txnRevNo: doc.TxnRevno,
			ID:       ID,
			Name:     doc.Name,
		}
		details = append(details, corewatcher.SecretBackendRotateChange{
			ID:              ID,
			Name:            doc.Name,
			NextTriggerTime: doc.NextRotateTime.UTC(),
		})
	}
	return details, nil
}

func (w *secretBackendRotationWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	w.watcher.WatchCollection(secretBackendsRotateC, ch)
	defer w.watcher.UnwatchCollection(secretBackendsRotateC, ch)
	details, err := w.initial()
	if err != nil {
		return err
	}
	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case change, ok := <-ch:
			if !ok {
				return tomb.ErrDying
			}
			if details, err = w.merge(details, change); err != nil {
				return err
			}
			if len(details) > 0 {
				out = w.out
			}
		case out <- details:
			out = nil
			details = nil
		}
	}
}
