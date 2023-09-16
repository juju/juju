// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/core/network"
)

// CloudServicer represents the state of a CAAS service.
type CloudServicer interface {
	// ProviderId returns the id assigned to the service
	// by the cloud.
	ProviderId() string

	// Addresses returns the service addresses.
	Addresses() network.SpaceAddresses

	// Generation returns the service config generation.
	Generation() int64

	// DesiredScaleProtected indicates if current desired scale in application has been applied to the cluster.
	DesiredScaleProtected() bool
}

// CloudService is an implementation of CloudService.
type CloudService struct {
	st  *State
	doc cloudServiceDoc
}

type cloudServiceDoc struct {
	// DocID holds cloud service document key.
	DocID string `bson:"_id"`

	ProviderId string    `bson:"provider-id"`
	Addresses  []address `bson:"addresses"`

	// Generation is the version of current service configuration.
	// It prevents the scale updated to replicas of the older/previous generations of deployment/statefulset.
	// Currently only DesiredScale is versioned.
	Generation int64 `bson:"generation"`

	// DesiredScaleProtected indicates if the desired scale needs to be applied to k8s cluster.
	// It prevents the desired scale requested from CLI by user incidentally updated by
	// k8s cluster replicas before having a chance to be applied/deployed.
	DesiredScaleProtected bool `bson:"desired-scale-protected"`
}

func newCloudService(st *State, doc *cloudServiceDoc) *CloudService {
	svc := &CloudService{
		st:  st,
		doc: *doc,
	}
	return svc
}

// Id implements CloudServicer.
func (c *CloudService) Id() string {
	return c.st.localID(c.doc.DocID)
}

// ProviderId implements CloudServicer.
func (c *CloudService) ProviderId() string {
	return c.doc.ProviderId
}

// Addresses implements CloudServicer.
func (c *CloudService) Addresses() network.SpaceAddresses {
	return networkAddresses(c.doc.Addresses)
}

// Generation implements CloudServicer.
func (c *CloudService) Generation() int64 {
	return c.doc.Generation
}

// DesiredScaleProtected implements CloudServicer.
func (c *CloudService) DesiredScaleProtected() bool {
	return c.doc.DesiredScaleProtected
}

func (c *CloudService) cloudServiceDoc() (*cloudServiceDoc, error) {
	coll, closer := c.st.db().GetCollection(cloudServicesC)
	defer closer()

	var doc cloudServiceDoc
	err := coll.FindId(c.doc.DocID).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("cloud service %v", c.Id())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

// CloudService return the content of cloud service from the underlying state.
// It returns an error that satisfies errors.IsNotFound if the cloud service has been removed.
func (c *CloudService) CloudService() (*CloudService, error) {
	doc, err := c.cloudServiceDoc()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if doc == nil {
		return nil, errors.NotFoundf("cloud service %v", c.Id())
	}
	c.doc = *doc
	return c, nil
}

// Refresh refreshes the content of cloud service from the underlying state.
// It returns an error that satisfies errors.IsNotFound if the cloud service has been removed.
func (c *CloudService) Refresh() error {
	_, err := c.CloudService()
	return errors.Trace(err)
}

func buildCloudServiceOps(st *State, doc cloudServiceDoc) ([]txn.Op, error) {
	svc := newCloudService(st, &doc)
	existing, err := svc.cloudServiceDoc()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	if err != nil || existing == nil {
		return []txn.Op{{
			C:      cloudServicesC,
			Id:     doc.DocID,
			Assert: txn.DocMissing,
			Insert: doc,
		}}, nil
	}
	patchFields := bson.D{}
	addField := func(elm bson.DocElem) {
		patchFields = append(patchFields, elm)
	}
	if doc.ProviderId != "" {
		addField(bson.DocElem{"provider-id", doc.ProviderId})
	}
	if len(doc.Addresses) > 0 {
		addField(bson.DocElem{"addresses", doc.Addresses})
	}
	if doc.Generation > existing.Generation {
		addField(bson.DocElem{"generation", doc.Generation})
	}
	if doc.DesiredScaleProtected != existing.DesiredScaleProtected {
		addField(bson.DocElem{"desired-scale-protected", doc.DesiredScaleProtected})
	}
	return []txn.Op{{
		C:  cloudServicesC,
		Id: existing.DocID,
		Assert: bson.D{{"$or", []bson.D{
			{{"provider-id", existing.ProviderId}},
			{{"provider-id", bson.D{{"$exists", false}}}},
		}}},
		Update: bson.D{
			{"$set", patchFields},
		},
	}}, nil
}

func (a *Application) removeCloudServiceOps() []txn.Op {
	ops := []txn.Op{{
		C:      cloudServicesC,
		Id:     a.globalKey(),
		Remove: true,
	}}
	return ops
}
