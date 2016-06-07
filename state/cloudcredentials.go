// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/cloud"
)

// cloudCredentialsDoc records information about a user's cloud credentials.
type cloudCredentialsDoc struct {
	DocID            string                            `bson:"_id"`
	CloudCredentials map[string]cloudCredentialsSubdoc `bson:"cloud-credentials,omitempty"`
}

// cloudCredentialsSubdoc records credentials and related information
// for a cloud.
type cloudCredentialsSubdoc struct {
	Credentials       map[string]cloudCredentialSubdoc `bson:"credentials"`
	DefaultCredential string                           `bson:"default-credential,omitempty"`
	DefaultRegion     string                           `bson:"default-region,omitempty"`
}

// cloudCredentialSubdoc records a credential for a cloud.
type cloudCredentialSubdoc struct {
	AuthType   string            `bson:"auth-type"`
	Label      string            `bson:"label,omitempty"`
	Attributes map[string]string `bson:"attributes"`
}

// initCloudCredentialsOps returns a list of txn.Ops that will initialize
// a set of cloud credentials for a user.
func initCloudCredentialsOps(user names.UserTag, cloudCredentials map[string]cloud.CloudCredential) []txn.Op {
	// TODO(axw) another document and collection to record references
	// to credentials from models. That would be needed to prevent
	// removal of credentials while models are still using them.
	id := names.NewUserTag(user.Canonical()).String()
	return []txn.Op{{
		C:      cloudCredentialsC,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: &cloudCredentialsDoc{
			CloudCredentials: makeCloudCredentials(cloudCredentials),
		},
	}}
}

func makeCloudCredentials(in map[string]cloud.CloudCredential) map[string]cloudCredentialsSubdoc {
	out := make(map[string]cloudCredentialsSubdoc)
	for cloudName, cloudCredential := range in {
		cloudCredentials := make(map[string]cloudCredentialSubdoc)
		for name, credential := range cloudCredential.AuthCredentials {
			cloudCredentials[name] = cloudCredentialSubdoc{
				string(credential.AuthType()),
				credential.Label,
				credential.Attributes(),
			}
		}
		out[cloudName] = cloudCredentialsSubdoc{
			cloudCredentials,
			cloudCredential.DefaultCredential,
			cloudCredential.DefaultRegion,
		}
	}
	return out
}

func (c cloudCredentialsSubdoc) toCloudCredential() cloud.CloudCredential {
	credentials := make(map[string]cloud.Credential)
	for name, credential := range c.Credentials {
		credentials[name] = credential.toCredential()
	}
	return cloud.CloudCredential{
		c.DefaultCredential,
		c.DefaultRegion,
		credentials,
	}
}

func (c cloudCredentialSubdoc) toCredential() cloud.Credential {
	out := cloud.NewCredential(cloud.AuthType(c.AuthType), c.Attributes)
	out.Label = c.Label
	return out
}

// CloudCredentials returns the user's cloud credentials, keyed by cloud name.
func (st *State) CloudCredentials(user names.UserTag) (map[string]cloud.CloudCredential, error) {
	key := names.NewUserTag(user.Canonical()).String()
	credentials, err := st.getCloudCredentials(key)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get cloud credentials for %q")
	}
	return credentials, nil
}

func (st *State) getCloudCredentials(key string) (map[string]cloud.CloudCredential, error) {
	coll, cleanup := st.getCollection(cloudCredentialsC)
	defer cleanup()

	var d cloudCredentialsDoc
	err := coll.FindId(key).One(&d)
	if err != nil && err != mgo.ErrNotFound {
		return nil, errors.Trace(err)
	}

	cloudCredentials := make(map[string]cloud.CloudCredential, len(d.CloudCredentials))
	for name, subdoc := range d.CloudCredentials {
		cloudCredentials[name] = subdoc.toCloudCredential()
	}
	return cloudCredentials, nil
}
