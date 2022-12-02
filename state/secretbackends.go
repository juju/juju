// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/mongo/utils"
)

// CreateSecretBackendParams are used to create a secret backend.
type CreateSecretBackendParams struct {
	Name                string
	Backend             string
	TokenRotateInterval *time.Duration
	Config              map[string]interface{}
}

// SecretBackendsStorage instances use mongo to store secret backend info.
type SecretBackendsStorage interface {
	CreateSecretBackend(params CreateSecretBackendParams) error
	ListSecretBackends() ([]*secrets.SecretBackend, error)
	GetSecretBackend(name string) (*secrets.SecretBackend, error)
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
	backend := &secretBackendDoc{
		DocID:               bson.NewObjectId().Hex(),
		Name:                p.Name,
		BackendType:         p.Backend,
		TokenRotateInterval: p.TokenRotateInterval,
		Config:              p.Config,
	}
	return backend, nil
}

// CreateSecretBackend creates a new secret backend.
func (s *secretBackendsStorage) CreateSecretBackend(p CreateSecretBackendParams) error {
	backendDoc, err := s.secretBackendDoc(&p)
	if err != nil {
		return errors.Trace(err)
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
	return errors.Trace(s.st.db().Run(buildTxn))
}

func (s *secretBackendsStorage) toSecretBackend(doc *secretBackendDoc) *secrets.SecretBackend {
	return &secrets.SecretBackend{
		Name:                doc.Name,
		BackendType:         doc.BackendType,
		TokenRotateInterval: doc.TokenRotateInterval,
		Config:              doc.Config,
	}
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
