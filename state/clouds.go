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

const publicCloudsKey = "public"

// cloudsDoc records information about a set of clouds.
type cloudsDoc struct {
	DocID  string                 `bson:"_id"`
	Clouds map[string]cloudSubdoc `bson:"clouds,omitempty"`
}

// cloudSubdoc records information about a cloud.
type cloudSubdoc struct {
	Type            string                 `bson:"type"`
	AuthTypes       []string               `bson:"auth-types,omitempty"`
	Endpoint        string                 `bson:"endpoint,omitempty"`
	StorageEndpoint string                 `bson:"storage-endpoint,omitempty"`
	Regions         []cloudRegionSubdoc    `bson:"regions,omitempty"`
	Config          map[string]interface{} `bson:"config,omitempty"`
}

// cloudRegionSubdoc records information about a cloud region.
type cloudRegionSubdoc struct {
	Name            string `bson:"name"`
	Endpoint        string `bson:"endpoint,omitempty"`
	StorageEndpoint string `bson:"storage-endpoint,omitempty"`
}

// initPublicCloudsOps returns a list of txn.Ops that will
// initialize the public clouds for the controller.
func initPublicCloudsOps(clouds map[string]cloud.Cloud) []txn.Op {
	id := publicCloudsKey
	return initCloudsOps(id, clouds)
}

// initPersonalCloudsOps returns a list of txn.Ops that will
// initialize the personal clouds for a given user.
func initPersonalCloudsOps(user names.UserTag, clouds map[string]cloud.Cloud) []txn.Op {
	id := names.NewUserTag(user.Canonical()).String()
	return initCloudsOps(id, clouds)
}

// initCloudsOps returns a list of txn.Ops that will initialize
// a set of clouds.
func initCloudsOps(id string, clouds map[string]cloud.Cloud) []txn.Op {
	// TODO(axw) add another document to record references to clouds
	// from models. That would be needed to prevent removal of clouds
	// while models are still using them.
	return []txn.Op{{
		C:      cloudsC,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: &cloudsDoc{Clouds: makeClouds(clouds)},
	}}
}

func makeClouds(in map[string]cloud.Cloud) map[string]cloudSubdoc {
	out := make(map[string]cloudSubdoc)
	for name, cloud := range in {
		authTypes := make([]string, len(cloud.AuthTypes))
		for i, authType := range cloud.AuthTypes {
			authTypes[i] = string(authType)
		}
		regions := make([]cloudRegionSubdoc, len(cloud.Regions))
		for i, region := range cloud.Regions {
			regions[i] = cloudRegionSubdoc{
				region.Name,
				region.Endpoint,
				region.StorageEndpoint,
			}
		}
		out[name] = cloudSubdoc{
			cloud.Type,
			authTypes,
			cloud.Endpoint,
			cloud.StorageEndpoint,
			regions,
			cloud.Config,
		}
	}
	return out
}

func (c cloudSubdoc) toCloud() cloud.Cloud {
	authTypes := make([]cloud.AuthType, len(c.AuthTypes))
	for i, authType := range c.AuthTypes {
		authTypes[i] = cloud.AuthType(authType)
	}
	regions := make([]cloud.Region, len(c.Regions))
	for i, region := range c.Regions {
		regions[i] = region.toRegion()
	}
	return cloud.Cloud{
		c.Type,
		authTypes,
		c.Endpoint,
		c.StorageEndpoint,
		regions,
		c.Config,
	}
}

func (r cloudRegionSubdoc) toRegion() cloud.Region {
	return cloud.Region{
		r.Name,
		r.Endpoint,
		r.StorageEndpoint,
	}
}

// PublicClouds returns information about all public clouds known to the
// controller.
func (st *State) PublicClouds() (map[string]cloud.Cloud, error) {
	clouds, err := st.getClouds(publicCloudsKey)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get public clouds")
	}
	return clouds, nil
}

// PersonalClouds returns information about personal clouds known to the
// controller. Each user may have their own set of personal clouds.
func (st *State) PersonalClouds(user names.UserTag) (map[string]cloud.Cloud, error) {
	key := names.NewUserTag(user.Canonical()).String()
	clouds, err := st.getClouds(key)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get clouds for %s", user.String())
	}
	return clouds, nil
}

func (st *State) getClouds(key string) (map[string]cloud.Cloud, error) {
	coll, cleanup := st.getCollection(cloudsC)
	defer cleanup()

	var d cloudsDoc
	err := coll.FindId(key).One(&d)
	if err != nil && err != mgo.ErrNotFound {
		return nil, errors.Trace(err)
	}

	clouds := make(map[string]cloud.Cloud, len(d.Clouds))
	for name, subdoc := range d.Clouds {
		cloud := subdoc.toCloud()
		clouds[name] = cloud
	}
	return clouds, nil
}
