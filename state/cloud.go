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
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/permission"
)

// cloudGlobalKey will return the key for a given cloud.
func cloudGlobalKey(cloudName string) string {
	return fmt.Sprintf("cloud#%s", cloudName)
}

// cloudDoc records information about the cloud that the controller operates in.
type cloudDoc struct {
	DocID            string                       `bson:"_id"`
	Name             string                       `bson:"name"`
	Type             string                       `bson:"type"`
	AuthTypes        []string                     `bson:"auth-types"`
	Endpoint         string                       `bson:"endpoint"`
	IdentityEndpoint string                       `bson:"identity-endpoint,omitempty"`
	StorageEndpoint  string                       `bson:"storage-endpoint,omitempty"`
	Regions          map[string]cloudRegionSubdoc `bson:"regions,omitempty"`
	CACertificates   []string                     `bson:"ca-certificates,omitempty"`
}

// cloudRegionSubdoc records information about cloud regions.
type cloudRegionSubdoc struct {
	Endpoint         string `bson:"endpoint,omitempty"`
	IdentityEndpoint string `bson:"identity-endpoint,omitempty"`
	StorageEndpoint  string `bson:"storage-endpoint,omitempty"`
}

// createCloudOp returns a txn.Op that will initialize
// the cloud definition for the controller.
func createCloudOp(cloud cloud.Cloud) txn.Op {
	authTypes := make([]string, len(cloud.AuthTypes))
	for i, authType := range cloud.AuthTypes {
		authTypes[i] = string(authType)
	}
	regions := make(map[string]cloudRegionSubdoc)
	for _, region := range cloud.Regions {
		regions[utils.EscapeKey(region.Name)] = cloudRegionSubdoc{
			region.Endpoint,
			region.IdentityEndpoint,
			region.StorageEndpoint,
		}
	}
	return txn.Op{
		C:      cloudsC,
		Id:     cloud.Name,
		Assert: txn.DocMissing,
		Insert: &cloudDoc{
			Name:             cloud.Name,
			Type:             cloud.Type,
			AuthTypes:        authTypes,
			Endpoint:         cloud.Endpoint,
			IdentityEndpoint: cloud.IdentityEndpoint,
			StorageEndpoint:  cloud.StorageEndpoint,
			Regions:          regions,
			CACertificates:   cloud.CACertificates,
		},
	}
}

// cloudModelRefCountKey returns a key for refcounting models
// for the specified cloud. Each time a model for the cloud is created,
// the refcount is incremented, and the opposite happens on removal.
func cloudModelRefCountKey(cloudName string) string {
	return fmt.Sprintf("cloudModel#%s", cloudName)
}

// incApplicationOffersRefOp returns a txn.Op that increments the reference
// count for a cloud model.
func incCloudModelRefOp(mb modelBackend, cloudName string) (txn.Op, error) {
	refcounts, closer := mb.db().GetCollection(globalRefcountsC)
	defer closer()
	cloudModelRefCountKey := cloudModelRefCountKey(cloudName)
	incRefOp, err := nsRefcounts.CreateOrIncRefOp(refcounts, cloudModelRefCountKey, 1)
	return incRefOp, errors.Trace(err)
}

// countCloudModelRefOp returns the number of models for a cloud,
// along with a txn.Op that ensures that that does not change.
func countCloudModelRefOp(mb modelBackend, cloudName string) (txn.Op, int, error) {
	refcounts, closer := mb.db().GetCollection(globalRefcountsC)
	defer closer()
	key := cloudModelRefCountKey(cloudName)
	return nsRefcounts.CurrentOp(refcounts, key)
}

// decCloudModelRefOp returns a txn.Op that decrements the reference
// count for a cloud model.
func decCloudModelRefOp(mb modelBackend, cloudName string) (txn.Op, error) {
	refcounts, closer := mb.db().GetCollection(globalRefcountsC)
	defer closer()
	cloudModelRefCountKey := cloudModelRefCountKey(cloudName)
	decRefOp, _, err := nsRefcounts.DyingDecRefOp(refcounts, cloudModelRefCountKey)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	return decRefOp, nil
}

func (d cloudDoc) toCloud() cloud.Cloud {
	authTypes := make([]cloud.AuthType, len(d.AuthTypes))
	for i, authType := range d.AuthTypes {
		authTypes[i] = cloud.AuthType(authType)
	}
	regionNames := make(set.Strings)
	for name := range d.Regions {
		regionNames.Add(name)
	}
	regions := make([]cloud.Region, len(d.Regions))
	for i, name := range regionNames.SortedValues() {
		region := d.Regions[name]
		regions[i] = cloud.Region{
			utils.UnescapeKey(name),
			region.Endpoint,
			region.IdentityEndpoint,
			region.StorageEndpoint,
		}
	}
	return cloud.Cloud{
		Name:             d.Name,
		Type:             d.Type,
		AuthTypes:        authTypes,
		Endpoint:         d.Endpoint,
		IdentityEndpoint: d.IdentityEndpoint,
		StorageEndpoint:  d.StorageEndpoint,
		Regions:          regions,
		CACertificates:   d.CACertificates,
	}
}

// Clouds returns the definitions for all clouds in the controller.
func (st *State) Clouds() (map[names.CloudTag]cloud.Cloud, error) {
	coll, cleanup := st.db().GetCollection(cloudsC)
	defer cleanup()

	var doc cloudDoc
	clouds := make(map[names.CloudTag]cloud.Cloud)
	iter := coll.Find(nil).Iter()
	for iter.Next(&doc) {
		clouds[names.NewCloudTag(doc.Name)] = doc.toCloud()
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Annotate(err, "getting clouds")
	}
	return clouds, nil
}

// Cloud returns the controller's cloud definition.
func (st *State) Cloud(name string) (cloud.Cloud, error) {
	coll, cleanup := st.db().GetCollection(cloudsC)
	defer cleanup()

	var doc cloudDoc
	err := coll.FindId(name).One(&doc)
	if err == mgo.ErrNotFound {
		return cloud.Cloud{}, errors.NotFoundf("cloud %q", name)
	}
	if err != nil {
		return cloud.Cloud{}, errors.Annotatef(err, "cannot get cloud %q", name)
	}
	return doc.toCloud(), nil
}

// AddCloud creates a cloud with the given name and details.
// Note that the Config is deliberately ignored - it's only
// relevant when bootstrapping.
func (st *State) AddCloud(c cloud.Cloud, owner string) error {
	if err := validateCloud(c); err != nil {
		return errors.Annotate(err, "invalid cloud")
	}
	ops := []txn.Op{createCloudOp(c)}
	if err := st.db().RunTransaction(ops); err != nil {
		if err == txn.ErrAborted {
			err = errors.AlreadyExistsf("cloud %q", c.Name)
		}
		return err
	}
	// Ensure the owner has access to the cloud.
	ownerTag := names.NewUserTag(owner)
	err := st.CreateCloudAccess(c.Name, ownerTag, permission.AdminAccess)
	if err != nil {
		return errors.Annotatef(err, "granting %s permission to the cloud owner", permission.AdminAccess)
	}

	return nil
}

// validateCloud checks that the supplied cloud is valid.
func validateCloud(cloud cloud.Cloud) error {
	if cloud.Name == "" {
		return errors.NotValidf("empty Name")
	}
	if cloud.Type == "" {
		return errors.NotValidf("empty Type")
	}
	if len(cloud.AuthTypes) == 0 {
		return errors.NotValidf("empty auth-types")
	}
	// TODO(axw) we should ensure that the cloud auth-types is a subset
	// of the auth-types supported by the provider. To do that, we'll
	// need a new "policy".
	return nil
}

// regionSettingsGlobalKey concatenates the cloud a hash and the region string.
func regionSettingsGlobalKey(cloud, region string) string {
	return cloud + "#" + region
}

// RemoveCloud removes a cloud and any credentials for that cloud.
// If the cloud is in use, ie has models deployed to it, the operation fails.
func (st *State) RemoveCloud(name string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if _, err := st.Cloud(name); err != nil {
			// Fail with not found error on first attempt if cloud doesn't exist.
			// On subsequent attempts, if cloud not found then
			// it was deleted by someone else and that's a no-op.
			if attempt > 1 && errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			}
			return nil, errors.Trace(err)
		}
		return st.removeCloudOps(name)
	}
	return st.db().Run(buildTxn)
}

// removeCloudOp returns a list of txn.Ops that will remove
// the specified cloud and any associated credentials.
func (st *State) removeCloudOps(name string) ([]txn.Op, error) {
	countOp, n, err := countCloudModelRefOp(st, name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if n != 0 {
		return nil, errors.Errorf("cloud is used by %d model%s", n, plural(n))
	}

	ops := []txn.Op{{
		C:      cloudsC,
		Id:     name,
		Remove: true,
	}, countOp}

	credPattern := bson.M{
		"_id": bson.M{"$regex": "^" + name + "#"},
	}
	credOps, err := st.removeInCollectionOps(cloudCredentialsC, credPattern)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, credOps...)

	permPattern := bson.M{
		"_id": bson.M{"$regex": "^" + cloudGlobalKey(name) + "#"},
	}
	permOps, err := st.removeInCollectionOps(permissionsC, permPattern)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ops = append(ops, permOps...)
	return ops, nil
}
