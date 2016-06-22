// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/cloud"
)

const controllerCloudKey = "controller"

// cloudGlobalKey returns the global database key for the specified cloud.
func cloudGlobalKey(name string) string {
	return "cloud#" + name
}

// cloudDoc records information about the cloud that the controller operates in.
type cloudDoc struct {
	DocID           string                       `bson:"_id"`
	Name            string                       `bson:"name"`
	Type            string                       `bson:"type"`
	AuthTypes       []string                     `bson:"auth-types"`
	Endpoint        string                       `bson:"endpoint"`
	StorageEndpoint string                       `bson:"storage-endpoint,omitempty"`
	Regions         map[string]cloudRegionSubdoc `bson:"regions,omitempty"`
}

// cloudRegionSubdoc records information about cloud regions.
type cloudRegionSubdoc struct {
	Endpoint        string `bson:"endpoint,omitempty"`
	StorageEndpoint string `bson:"storage-endpoint,omitempty"`
}

// createCloudOp returns a list of txn.Ops that will initialize
// the cloud definition for the controller.
func createCloudOp(cloud cloud.Cloud, cloudName string) txn.Op {
	authTypes := make([]string, len(cloud.AuthTypes))
	for i, authType := range cloud.AuthTypes {
		authTypes[i] = string(authType)
	}
	regions := make(map[string]cloudRegionSubdoc)
	for _, region := range cloud.Regions {
		regions[region.Name] = cloudRegionSubdoc{
			region.Endpoint,
			region.StorageEndpoint,
		}
	}
	return txn.Op{
		C:      controllersC,
		Id:     controllerCloudKey,
		Assert: txn.DocMissing,
		Insert: &cloudDoc{
			Name:            cloudName,
			Type:            cloud.Type,
			AuthTypes:       authTypes,
			Endpoint:        cloud.Endpoint,
			StorageEndpoint: cloud.StorageEndpoint,
			Regions:         regions,
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
			name,
			region.Endpoint,
			region.StorageEndpoint,
		}
	}
	return cloud.Cloud{
		d.Type,
		authTypes,
		d.Endpoint,
		d.StorageEndpoint,
		regions,
		nil, // Config is not stored, only relevant to bootstrap
	}
}

// Cloud returns the controller's cloud definition.
func (st *State) Cloud() (cloud.Cloud, error) {
	coll, cleanup := st.getCollection(controllersC)
	defer cleanup()

	var doc cloudDoc
	err := coll.FindId(controllerCloudKey).One(&doc)
	if err != nil {
		return cloud.Cloud{}, errors.Annotatef(err, "cannot get cloud definition")
	}
	return doc.toCloud(), nil
}

// validateCloud checks that the supplied cloud is valid.
func validateCloud(cloud cloud.Cloud) error {
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
