// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/mongo/utils"
)

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
	ModelCount       int                          `bson:"modelcount"`
}

// cloudRegionSubdoc records information about cloud regions.
type cloudRegionSubdoc struct {
	Endpoint         string `bson:"endpoint,omitempty"`
	IdentityEndpoint string `bson:"identity-endpoint,omitempty"`
	StorageEndpoint  string `bson:"storage-endpoint,omitempty"`
}

// createCloudOp returns a list of txn.Ops that will initialize
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
			ModelCount:       cloud.ModelCount,
		},
	}
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
		ModelCount:       d.ModelCount,
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
func (st *State) AddCloud(c cloud.Cloud) error {
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
	if cloud.ModelCount > 0 {
		return errors.NotValidf("model count (%d) > 0", cloud.ModelCount)
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

	checkUnused := func() error {
		c, err := st.Cloud(name)
		if errors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return errors.Trace(err)
		}
		if c.ModelCount > 0 {
			return errors.Errorf("cloud is used by %d model%s", c.ModelCount, plural(c.ModelCount))
		}
		return nil
	}
	// Check that the cloud is not used by any models.
	if err := checkUnused(); err != nil {
		return errors.Trace(err)
	}

	// Check that the cloud is not the primary controller cloud.
	// This shouldn't be needed since the above model check will
	// pick it up, but just in case....
	ci, err := st.ControllerInfo()
	if err != nil {
		return errors.Trace(err)
	}
	if ci.CloudName == name {
		return errors.Errorf("cloud is used by controller")
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			err := checkUnused()
			if err == nil {
				return nil, jujutxn.ErrNoOperations
			}
			return nil, errors.Trace(err)
		}
		ops := removeCloudOps(name)
		return ops, nil
	}
	if err := st.db().Run(buildTxn); err != nil {
		return err
	}

	// Cloud has been removed, now bulk remove any credentials for that cloud.
	// We could use removeInCollectionOps to generate a slice of ops to
	// use in the previous Run() call but on large systems there could be
	// many user credentials to remove and so a bulk op is better. There's
	// a small risk of orphaned credentials if an error occurs but there'll
	// be no adverse effects.
	return st.removeCloudCredentials(name)
}

// removeCloudOp returns a list of txn.Ops that will remove
// the specified cloud.
func removeCloudOps(name string) []txn.Op {
	ops := []txn.Op{{
		C:      cloudsC,
		Id:     name,
		Assert: bson.D{{"modelcount", 0}},
		Remove: true,
	}}
	return ops
}

func (st *State) removeCloudCredentials(cloud string) error {
	coll, closer := st.db().GetRawCollection(cloudCredentialsC)
	defer closer()

	bulk := coll.Bulk()
	bulk.Unordered()
	bulk.RemoveAll(bson.D{{"_id", bson.D{{"$regex", "^" + cloud + "#"}}}})
	_, err := bulk.Run()
	return errors.Annotate(err, "deleting cloud credentials")
}
