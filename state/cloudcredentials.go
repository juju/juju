// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/permission"
)

// Credential contains information about the credential as stored on
// the controller.
type Credential struct {
	cloudCredentialDoc
}

// CloudCredentialTag returns cloud credential tag.
func (c Credential) CloudCredentialTag() (names.CloudCredentialTag, error) {
	return c.cloudCredentialDoc.cloudCredentialTag()
}

// IsValid indicates whether the credential is valid.
func (c Credential) IsValid() bool {
	return !c.cloudCredentialDoc.Invalid
}

// cloudCredentialDoc records information about a user's cloud credentials.
type cloudCredentialDoc struct {
	DocID      string            `bson:"_id"`
	Owner      string            `bson:"owner"`
	Cloud      string            `bson:"cloud"`
	Name       string            `bson:"name"`
	Revoked    bool              `bson:"revoked"`
	AuthType   string            `bson:"auth-type"`
	Attributes map[string]string `bson:"attributes,omitempty"`

	// Invalid stores flag that indicates if a credential is invalid.
	// Note that the credential is valid:
	//  * if the flag is explicitly set to 'false'; or
	//  * if the flag is not set at all, as will be the case for
	//    new inserts or credentials created with previous Juju versions. In
	//    this case, we'd still read it as 'false' and the credential validity
	//    will be interpreted correctly.
	// This flag will need to be explicitly set to 'true' for a credential
	// to be considered invalid.
	Invalid bool `bson:"invalid"`

	// InvalidReason contains the reason why the credential was marked as invalid.
	// This can range from cloud messages such as an expired credential to
	// commercial reasons set via CLI or api calls.
	InvalidReason string `bson:"invalid-reason,omitempty"`
}

// CloudCredential returns the cloud credential for the given tag.
func (st *State) CloudCredential(tag names.CloudCredentialTag) (Credential, error) {
	coll, cleanup := st.db().GetCollection(cloudCredentialsC)
	defer cleanup()

	var doc cloudCredentialDoc
	err := coll.FindId(cloudCredentialDocID(tag)).One(&doc)
	if err == mgo.ErrNotFound {
		return Credential{}, errors.NotFoundf(
			"cloud credential %q", tag.Id(),
		)
	} else if err != nil {
		return Credential{}, errors.Annotatef(
			err, "getting cloud credential %q", tag.Id(),
		)
	}
	return Credential{doc}, nil
}

// CloudCredentials returns the user's cloud credentials for a given cloud,
// keyed by credential name.
func (st *State) CloudCredentials(user names.UserTag, cloudName string) (map[string]Credential, error) {
	coll, cleanup := st.db().GetCollection(cloudCredentialsC)
	defer cleanup()

	credentials := make(map[string]Credential)
	iter := coll.Find(bson.D{
		{"owner", user.Id()},
		{"cloud", cloudName},
	}).Iter()
	defer iter.Close()

	var doc cloudCredentialDoc
	for iter.Next(&doc) {
		tag, err := doc.cloudCredentialTag()
		if err != nil {
			return nil, errors.Trace(err)
		}
		credentials[tag.Id()] = Credential{doc}
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Annotatef(
			err, "cannot get cloud credentials for user %q, cloud %q",
			user.Id(), cloudName,
		)
	}
	return credentials, nil
}

// UpdateCloudCredential adds or updates a cloud credential with the given tag.
func (st *State) UpdateCloudCredential(tag names.CloudCredentialTag, credential cloud.Credential) error {
	credentials := map[names.CloudCredentialTag]Credential{
		tag: convertCloudCredentialToState(tag, credential),
	}
	annotationMsg := "updating cloud credentials"
	buildTxn := func(attempt int) ([]txn.Op, error) {
		cloudName := tag.Cloud().Id()
		cloud, err := st.Cloud(cloudName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops, err := validateCloudCredentials(cloud, credentials)
		if err != nil {
			return nil, errors.Trace(err)
		}
		_, err = st.CloudCredential(tag)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Maskf(err, "fetching cloud credentials")
		}
		if err == nil {
			ops = append(ops, updateCloudCredentialOp(tag, credential))
		} else {
			annotationMsg = "creating cloud credential"
			if credential.Invalid || credential.InvalidReason != "" {
				return nil, errors.NotSupportedf("adding invalid credential")
			}
			ops = append(ops, createCloudCredentialOp(tag, credential))
		}
		return ops, nil
	}
	if err := st.db().Run(buildTxn); err != nil {
		return errors.Annotate(err, annotationMsg)
	}
	return nil
}

// InvalidateCloudCredential marks a cloud credential with the given tag as invalid.
func (st *State) InvalidateCloudCredential(tag names.CloudCredentialTag, reason string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		_, err := st.CloudCredential(tag)
		if err != nil {
			return nil, errors.Trace(err)
		}

		ops := []txn.Op{{
			C:      cloudCredentialsC,
			Id:     cloudCredentialDocID(tag),
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"invalid", true},
				{"invalid-reason", reason},
			}}},
		}}
		return ops, nil
	}
	if err := st.db().Run(buildTxn); err != nil {
		return errors.Annotatef(err, "invalidating cloud credential %v", tag.Id())
	}
	return nil
}

// RemoveCloudCredential removes a cloud credential with the given tag.
func (st *State) RemoveCloudCredential(tag names.CloudCredentialTag) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		_, err := st.CloudCredential(tag)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		return removeCloudCredentialOps(tag), nil
	}
	if err := st.db().Run(buildTxn); err != nil {
		return errors.Annotate(err, "removing cloud credential")
	}
	return nil
}

// createCloudCredentialOp returns a txn.Op that will create
// a cloud credential.
func createCloudCredentialOp(tag names.CloudCredentialTag, cred cloud.Credential) txn.Op {
	return txn.Op{
		C:      cloudCredentialsC,
		Id:     cloudCredentialDocID(tag),
		Assert: txn.DocMissing,
		Insert: &cloudCredentialDoc{
			Owner:      tag.Owner().Id(),
			Cloud:      tag.Cloud().Id(),
			Name:       tag.Name(),
			AuthType:   string(cred.AuthType()),
			Attributes: cred.Attributes(),
			Revoked:    cred.Revoked,
		},
	}
}

// updateCloudCredentialOp returns a txn.Op that will update
// a cloud credential.
func updateCloudCredentialOp(tag names.CloudCredentialTag, cred cloud.Credential) txn.Op {
	return txn.Op{
		C:      cloudCredentialsC,
		Id:     cloudCredentialDocID(tag),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{
			{"auth-type", string(cred.AuthType())},
			{"attributes", cred.Attributes()},
			{"revoked", cred.Revoked},
			{"invalid", cred.Invalid},
			{"invalid-reason", cred.InvalidReason},
		}}},
	}
}

// removeCloudCredentialOp returns a txn.Op that will remove
// a cloud credential.
func removeCloudCredentialOps(tag names.CloudCredentialTag) []txn.Op {
	return []txn.Op{{
		C:      cloudCredentialsC,
		Id:     cloudCredentialDocID(tag),
		Assert: txn.DocExists,
		Remove: true,
	}}
}

func cloudCredentialDocID(tag names.CloudCredentialTag) string {
	return fmt.Sprintf("%s#%s#%s", tag.Cloud().Id(), tag.Owner().Id(), tag.Name())
}

func (c cloudCredentialDoc) cloudCredentialTag() (names.CloudCredentialTag, error) {
	ownerTag := names.NewUserTag(c.Owner)
	id := fmt.Sprintf("%s/%s/%s", c.Cloud, ownerTag.Id(), c.Name)
	if !names.IsValidCloudCredential(id) {
		return names.CloudCredentialTag{}, errors.NotValidf("cloud credential ID")
	}
	return names.NewCloudCredentialTag(id), nil
}

// validateCloudCredentials checks that the supplied cloud credentials are
// valid for use with the controller's cloud, and returns a set of txn.Ops
// to assert the same in a transaction. The map keys are the cloud credential
// IDs.
//
// TODO(rogpeppe) We're going to a lot of effort here to assert that a
// cloud's auth types haven't changed since we looked at them a moment
// ago, but we don't support changing a cloud's definition currently and
// it's not clear that doing so would be a good idea, as changing a
// cloud's auth type would invalidate all existing credentials and would
// usually involve a new provider version and juju binary too, so
// perhaps all this code is unnecessary.
func validateCloudCredentials(
	cloud cloud.Cloud,
	credentials map[names.CloudCredentialTag]Credential,
) ([]txn.Op, error) {
	requiredAuthTypes := make(set.Strings)
	for tag, credential := range credentials {
		err := validateCredentialForCloud(cloud, tag, credential)
		if err != nil {
			return nil, errors.Annotatef(err, "validating credential %q for cloud %q", tag.Id(), cloud.Name)
		}
		requiredAuthTypes.Add(credential.AuthType)
	}
	ops := make([]txn.Op, len(requiredAuthTypes))
	for i, authType := range requiredAuthTypes.SortedValues() {
		ops[i] = txn.Op{
			C:      cloudsC,
			Id:     cloud.Name,
			Assert: bson.D{{"auth-types", authType}},
		}
	}
	return ops, nil
}

func validateCredentialForCloud(nuage cloud.Cloud, tag names.CloudCredentialTag, credential Credential) error {
	if tag.Cloud().Id() != nuage.Name {
		return errors.NotValidf("cloud %q", tag.Cloud().Id())
	}

	supportedAuth := func() bool {
		for _, authType := range nuage.AuthTypes {
			if credential.AuthType == string(authType) {
				return true
			}
		}
		return false
	}

	if !supportedAuth() {
		return errors.NotSupportedf("supported auth-types %q, %q", nuage.AuthTypes, credential.AuthType)
	}
	return nil
}

// WatchCredential returns a new NotifyWatcher watching for
// changes to the specified credential.
func (st *State) WatchCredential(cred names.CloudCredentialTag) NotifyWatcher {
	filter := func(rawId interface{}) bool {
		id, ok := rawId.(string)
		if !ok {
			return false
		}
		return id == cloudCredentialDocID(cred)
	}
	return newNotifyCollWatcher(st, cloudCredentialsC, filter)
}

// AllCloudCredentials returns all cloud credentials stored on the controller
// for a given user.
func (st *State) AllCloudCredentials(user names.UserTag) ([]Credential, error) {
	coll, cleanup := st.db().GetCollection(cloudCredentialsC)
	defer cleanup()

	// There are 2 ways of getting a credential for a user:
	// 1. user name stored in the credential tag (aka doc id);
	// 2. look up using Owner field.
	// We use Owner field below as credential tag or doc ID may be changed
	// in the future to be a real Primary Key that has nothing to do with
	// the data it identifies, i.e. no business meaning.
	clause := bson.D{{"owner", user.Id()}}

	var docs []cloudCredentialDoc
	err := coll.Find(clause).Sort("cloud").All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "getting cloud credentials for %q", user.Id())
	}

	if len(docs) == 0 {
		return nil, errors.NotFoundf("cloud credentials for %q", user.Id())
	}

	credentials := make([]Credential, len(docs))
	for i, doc := range docs {
		credentials[i] = Credential{doc}
	}
	return credentials, nil
}

// CredentialModels returns all models that use given cloud credential.
func (st *State) CredentialModels(tag names.CloudCredentialTag) (map[string]string, error) {
	coll, cleanup := st.db().GetCollection(modelsC)
	defer cleanup()

	sel := bson.D{
		{"cloud-credential", tag.Id()},
		{"life", bson.D{{"$ne", Dead}}},
	}

	var docs []modelDoc
	err := coll.Find(sel).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "getting models that use cloud credential %q", tag.Id())
	}
	if len(docs) == 0 {
		return nil, errors.NotFoundf("models that use cloud credentials %q", tag.Id())
	}

	results := make(map[string]string, len(docs))
	for _, model := range docs {
		results[model.UUID] = model.Name
	}
	return results, nil
}

// CredentialOwnerModelAccess stores cloud credential model information for the credential owner
// or an error retrieving it.
type CredentialOwnerModelAccess struct {
	ModelUUID   string
	ModelName   string
	OwnerAccess permission.Access
	Error       error
}

// CredentialModelsAndOwnerAccess returns all models that use given cloud credential as well as
// what access the credential owner has on these models.
func (st *State) CredentialModelsAndOwnerAccess(tag names.CloudCredentialTag) ([]CredentialOwnerModelAccess, error) {
	models, err := st.CredentialModels(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []CredentialOwnerModelAccess
	for uuid, name := range models {
		ownerAccess, err := st.UserAccess(tag.Owner(), names.NewModelTag(uuid))
		if err != nil {
			if errors.IsNotFound(err) {
				results = append(results, CredentialOwnerModelAccess{ModelName: name, ModelUUID: uuid, OwnerAccess: permission.NoAccess})
				continue
			}
			results = append(results, CredentialOwnerModelAccess{ModelName: name, ModelUUID: uuid, Error: errors.Trace(err)})
			continue
		}
		results = append(results, CredentialOwnerModelAccess{ModelName: name, ModelUUID: uuid, OwnerAccess: ownerAccess.Access})
	}
	return results, nil
}
