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

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/mongo/utils"
)

// CreateSecretBackendParams are used to create a secret backend.
type CreateSecretBackendParams struct {
	ID                  string
	Name                string
	BackendType         string
	TokenRotateInterval *time.Duration
	Config              map[string]interface{}
}

// UpdateSecretBackendParams are used to update a secret backend.
type UpdateSecretBackendParams struct {
	ID                  string
	NameChange          *string
	TokenRotateInterval *time.Duration
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
			if !errors.IsNotFound(err) {
				return nil, errors.Annotatef(err, "checking for existing secret backend")
			}
		} else {
			return nil, errors.AlreadyExistsf("secret backend %q", p.Name)
		}
		return []txn.Op{{
			C:      secretBackendsC,
			Id:     backendDoc.DocID,
			Assert: txn.DocMissing,
			Insert: *backendDoc,
		}}, nil
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
		// This isn't perfect but we don't want to use the name as the doc id.
		// The tiny window for multiple callers to create dupe backends will
		// go away once we transition to a SQL backend.
		if p.NameChange != nil {
			if existing, err := s.GetSecretBackend(doc.Name); err != nil {
				if !errors.IsNotFound(err) {
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
		return []txn.Op{{
			C:      secretBackendsC,
			Id:     doc.DocID,
			Assert: txn.DocExists,
			Update: update,
		}}, nil
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
			if errors.IsNotFound(err) {
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
		// If we are forcing removal, simply delete any ref count reference.
		removeRefCountOp := s.st.removeBackendRevisionCountOp(b.ID)
		if force {
			return []txn.Op{deleteOp, removeRefCountOp}, nil
		}
		// Check that there are no revisions stored in the backend before allowing deletion.
		ensureRefCountOp, count, err := s.st.ensureSecretBackendRevisionCountOp(b.ID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if count > 0 {
			return nil, errors.NotSupportedf("removing backend with %d stored secret revisions", count)
		}
		return []txn.Op{deleteOp, ensureRefCountOp}, nil
	}
	return errors.Trace(s.st.db().Run(buildTxn))
}

func secretBackendRefCountKey(backendID string) string {
	return fmt.Sprintf("secretbackend#revisions#%s", backendID)
}

func (st *State) ensureSecretBackendRevisionCountOp(backendID string) (txn.Op, int, error) {
	refCountCollection, ccloser := st.db().GetCollection(globalRefcountsC)
	defer ccloser()

	return nsRefcounts.CurrentOp(refCountCollection, secretBackendRefCountKey(backendID))
}

func (st *State) removeBackendRevisionCountOp(backendID string) txn.Op {
	return nsRefcounts.JustRemoveOp(globalRefcountsC, secretBackendRefCountKey(backendID), -1)
}

// incBackendRevisionCountOps returns the ops needed to change the secret revision ref count
// for the specified backend. Used to ensure backends with revisions cannot be deleted without force.
func (st *State) incBackendRevisionCountOps(backendID string) ([]txn.Op, error) {
	refCountCollection, ccloser := st.db().GetCollection(globalRefcountsC)
	defer ccloser()

	key := secretBackendRefCountKey(backendID)
	countOp, _, err := nsRefcounts.CurrentOp(refCountCollection, key)
	if err != nil {
		return nil, errors.Trace(err)
	}
	incOp, err := nsRefcounts.CreateOrIncRefOp(refCountCollection, key, 1)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return []txn.Op{countOp, incOp}, nil
}
