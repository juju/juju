// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/mongo/utils"
)

// CreateSecretParams are used to create a secret.
// TODO(wallyworld) - add tags and description etc
type CreateSecretParams struct {
	ControllerUUID string
	ModelUUID      string
	ProviderLabel  string
	Version        int
	Type           string
	Path           string
	RotateInterval time.Duration
	Params         map[string]interface{}
	Data           secrets.SecretData
}

// UpdateSecretParams are used to update a secret.
// TODO(wallyworld) - add tags and description etc
type UpdateSecretParams struct {
	RotateInterval time.Duration
	Params         map[string]interface{}
	Data           secrets.SecretData
}

// TODO(wallyworld)
type SecretsFilter struct{}

// SecretsStore instances use mongo as a secrets store.
type SecretsStore interface {
	CreateSecret(p CreateSecretParams) (*secrets.SecretMetadata, error)
	UpdateSecret(URL *secrets.URL, p UpdateSecretParams) (*secrets.SecretMetadata, error)
	GetSecretValue(URL *secrets.URL) (secrets.SecretValue, error)
	ListSecrets(filter SecretsFilter) ([]*secrets.SecretMetadata, error)
}

// NewSecretsStore creates a new mongo backed secrets store.
func NewSecretsStore(st *State) *secretsStore {
	return &secretsStore{st: st}
}

type secretMetadataDoc struct {
	DocID string `bson:"_id"`

	Path           string            `bson:"path"`
	Version        int               `bson:"version"`
	RotateInterval time.Duration     `bson:"rotate-interval"`
	Description    string            `bson:"description"`
	Tags           map[string]string `bson:"tags"`
	ID             int               `bson:"id"`
	Provider       string            `bson:"provider"`
	ProviderID     string            `bson:"provider-id"`
	Revision       int               `bson:"revision"`
	CreateTime     time.Time         `bson:"create-time"`
	UpdateTime     time.Time         `bson:"update-time"`
}

type secretValueDoc struct {
	DocID string `bson:"_id"`

	Data secretsDataMap `bson:"data"`
}

type secretsDataMap map[string]interface{}

func (m *secretsDataMap) SetBSON(raw bson.Raw) error {
	rawMap := make(map[string]interface{})
	if err := raw.Unmarshal(rawMap); err != nil {
		return err
	}
	*m = utils.UnescapeKeys(rawMap)
	return nil
}

func (m secretsDataMap) GetBSON() (interface{}, error) {
	escapedMap := utils.EscapeKeys(m)
	return escapedMap, nil
}

type secretsStore struct {
	st *State
}

func (s *secretsStore) secretMetadataDoc(URL *secrets.URL, p *CreateSecretParams) (*secretMetadataDoc, error) {
	id, err := sequenceWithMin(s.st, "secret", 1)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &secretMetadataDoc{
		DocID:          URL.String(),
		Path:           p.Path,
		Version:        p.Version,
		RotateInterval: p.RotateInterval,
		Description:    "",
		Tags:           nil,
		ID:             id,
		Provider:       p.ProviderLabel,
		ProviderID:     "",
		Revision:       1,
		CreateTime:     s.st.nowToTheSecond(),
		UpdateTime:     s.st.nowToTheSecond(),
	}, nil
}

func (s *secretsStore) updateSecretMetadataDoc(doc *secretMetadataDoc, p *UpdateSecretParams) {
	doc.UpdateTime = s.st.nowToTheSecond()
	doc.RotateInterval = p.RotateInterval
	doc.Revision = doc.Revision + 1
}

func (s *secretsStore) secretValueDoc(url *secrets.URL, data secrets.SecretData) *secretValueDoc {
	dataCopy := make(secretsDataMap)
	for k, v := range data {
		dataCopy[k] = v
	}
	return &secretValueDoc{
		DocID: url.String(),
		Data:  dataCopy,
	}
}

// CreateSecret creates a new secret.
func (s *secretsStore) CreateSecret(p CreateSecretParams) (*secrets.SecretMetadata, error) {
	URL := &secrets.URL{
		Version:        fmt.Sprintf("v%d", p.Version),
		ControllerUUID: p.ControllerUUID,
		ModelUUID:      p.ModelUUID,
		Path:           p.Path,
		Revision:       1,
	}
	metadataDoc, err := s.secretMetadataDoc(URL, &p)
	if err != nil {
		return nil, errors.Trace(err)
	}
	valueDoc := s.secretValueDoc(URL, p.Data)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if _, err := s.GetSecretValue(URL); err == nil {
				return nil, errors.AlreadyExistsf("secret value for %q", URL.ID())
			}
		}
		ops := []txn.Op{
			{
				C:      secretMetadataC,
				Id:     metadataDoc.DocID,
				Assert: txn.DocMissing,
				Insert: *metadataDoc,
			}, {
				C:      secretValuesC,
				Id:     valueDoc.DocID,
				Assert: txn.DocMissing,
				Insert: *valueDoc,
			},
		}
		return ops, nil
	}
	err = s.st.db().Run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	metadata := s.toSecretMetadata(metadataDoc)
	metadata.URL = URL
	return metadata, nil
}

// UpdateSecret updates an existing secret.
func (s *secretsStore) UpdateSecret(URL *secrets.URL, p UpdateSecretParams) (*secrets.SecretMetadata, error) {
	if URL.Revision > 0 {
		return nil, errors.New("cannot specify a revision when updating a secret")
	}
	secretMetadataCollection, closer := s.st.db().GetCollection(secretMetadataC)
	defer closer()

	var metadataDoc secretMetadataDoc
	baseURL := URL.WithRevision(0)
	updatedURL := *URL
	buildTxn := func(attempt int) ([]txn.Op, error) {
		err := secretMetadataCollection.FindId(baseURL.ID()).One(&metadataDoc)
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, errors.NotFoundf("secret %q", baseURL.ID())
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		s.updateSecretMetadataDoc(&metadataDoc, &p)
		updatedURL = *updatedURL.WithRevision(metadataDoc.Revision)
		if _, err := s.GetSecretValue(&updatedURL); err == nil {
			return nil, errors.AlreadyExistsf("secret value for %q", URL.ID())
		}

		valueDoc := s.secretValueDoc(&updatedURL, p.Data)
		ops := []txn.Op{
			{
				C:      secretMetadataC,
				Id:     metadataDoc.DocID,
				Assert: txn.DocExists,
				Update: bson.M{"$set": metadataDoc},
			}, {
				C:      secretValuesC,
				Id:     valueDoc.DocID,
				Assert: txn.DocMissing,
				Insert: *valueDoc,
			},
		}
		return ops, nil
	}
	err := s.st.db().Run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	metadata := s.toSecretMetadata(&metadataDoc)
	metadata.URL = &updatedURL
	return metadata, nil
}

func (s *secretsStore) toSecretMetadata(doc *secretMetadataDoc) *secrets.SecretMetadata {
	return &secrets.SecretMetadata{
		Path:           doc.Path,
		Version:        doc.Version,
		RotateInterval: doc.RotateInterval,
		Description:    doc.Description,
		Tags:           doc.Tags,
		ID:             doc.ID,
		Provider:       doc.Provider,
		ProviderID:     doc.ProviderID,
		Revision:       doc.Revision,
		CreateTime:     doc.CreateTime,
		UpdateTime:     doc.UpdateTime,
	}
}

// GetSecretValue gets the secret value for the specified URL.
func (s *secretsStore) GetSecretValue(URL *secrets.URL) (secrets.SecretValue, error) {
	secretValuesCollection, closer := s.st.db().GetCollection(secretValuesC)
	defer closer()

	var doc secretValueDoc
	err := secretValuesCollection.FindId(URL.ID()).One(&doc)
	if errors.Cause(err) == mgo.ErrNotFound {
		return nil, errors.NotFoundf("secret %q", URL.String())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	data := make(secrets.SecretData)
	for k, v := range doc.Data {
		if URL.Attribute != "" && k != URL.Attribute {
			continue
		}
		data[k] = fmt.Sprintf("%v", v)
	}
	if URL.Attribute != "" && len(data) == 0 {
		return nil, errors.NotFoundf("secret attribute %q", URL.Attribute)
	}
	return secrets.NewSecretValue(data), nil
}

// ListSecrets list the secrets using the specified filter.
// TODO(wallywolrd) - implement filter
func (s *secretsStore) ListSecrets(filter SecretsFilter) ([]*secrets.SecretMetadata, error) {
	secretMetadataCollection, closer := s.st.db().GetCollection(secretMetadataC)
	defer closer()

	var docs []secretMetadataDoc
	// TODO(wallywolrd) - use filter
	err := secretMetadataCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]*secrets.SecretMetadata, len(docs))
	for i, doc := range docs {
		result[i] = s.toSecretMetadata(&doc)
	}
	return result, nil
}
