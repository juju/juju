// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/network"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// CloudServicer represents the state of a CAAS service.
type CloudServicer interface {
	// ProviderId returns the id assigned to the service
	// by the cloud.
	ProviderId() string

	// Addresses returns the service addresses.
	Addresses() []network.Address

	// GetScale returns the application's desired scale value.
	GetScale() int

	// // DesiredScaleApplied confirms the desired scale has been applied to the cluster.
	// DesiredScaleApplied() (int, error)

	// ChangeScale alters the existing scale by the provided change amount, returning the new amount.
	ChangeScale(scaleChange int) (int, error)

	// SetScale sets the application's desired scale value.
	SetScale(scale int, generation int64, force bool) error
}

// CloudService is an implementation of CloudService.
type CloudService struct {
	st  *State
	doc cloudServiceDoc
}

type cloudServiceDoc struct {
	// DocID holds cloud service document key.
	DocID string `bson:"_id"`

	ProviderId string `bson:"provider-id"`

	// generation is the version of current service configuration.
	// It prevents the scale updated to replicas of the older/previous gerenations of deployment/statefulset.
	// Currently only DesiredScale is versioned.
	Generation int64 `bson:"generation"`
	// CAAS related attributes.
	DesiredScale int `bson:"scale"`
	// DesiredScaleApplied indicates if the desired scale has been applied to k8s cluster.
	// It prevents the desired scale requested from CLI by user incidentally updated by
	// k8s cluster replicas before having a chance to be applied/deployed.
	DesiredScaleApplied bool `bson:"applied"`

	Addresses []address `bson:"addresses"`
}

func newCloudService(st *State, doc *cloudServiceDoc) *CloudService {
	svc := &CloudService{
		st:  st,
		doc: *doc,
	}
	return svc
}

// Id implements CloudService.
func (c *CloudService) Id() string {
	return c.doc.DocID
}

// ProviderId implements CloudService.
func (c *CloudService) ProviderId() string {
	return c.doc.ProviderId
}

// Addresses implements CloudService.
func (c *CloudService) Addresses() []network.Address {
	return networkAddresses(c.doc.Addresses)
}

func (c *CloudService) cloudServiceDoc() (*cloudServiceDoc, error) {
	coll, closer := c.st.db().GetCollection(cloudServicesC)
	defer closer()

	var doc cloudServiceDoc
	err := coll.FindId(c.Id()).One(&doc)
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

func (c *CloudService) saveServiceOps(doc cloudServiceDoc) ([]txn.Op, error) {
	existing, err := c.cloudServiceDoc()
	if err != nil && !errors.IsNotFound(err) {
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
	return []txn.Op{{
		C:  cloudServicesC,
		Id: existing.DocID,
		Assert: bson.D{{"$or", []bson.D{
			{{"provider-id", doc.ProviderId}},
			{{"provider-id", bson.D{{"$exists", false}}}},
		}}},
		Update: bson.D{
			{"$set",
				bson.D{
					{"provider-id", doc.ProviderId},
					{"addresses", doc.Addresses},
				},
			},
		},
	}}, nil
}

// GetScale returns the application's desired scale value.
// This is used on CAAS models.
func (c *CloudService) GetScale() int {
	return c.doc.DesiredScale
}

// // DesiredScaleApplied confirms the desired scale has been applied to the cluster.
// func (c *CloudService) DesiredScaleApplied() (int, error) {
// 	buildTxn := func(attempt int) ([]txn.Op, error) {
// 		if attempt > 0 {
// 			if err := c.Refresh(); err != nil {
// 				return nil, errors.Trace(err)
// 			}
// 			if c.doc.DesiredScaleApplied {
// 				// already applied, no ops.
// 				return nil, nil
// 			}
// 			alive, err := isAlive(c.st, applicationsC, c.doc.DocID)
// 			if err != nil {
// 				return nil, errors.Trace(err)
// 			} else if !alive {
// 				return nil, applicationNotAliveErr
// 			}
// 		}
// 		return []txn.Op{{
// 			C:  cloudServicesC,
// 			Id: c.doc.DocID,
// 			Assert: bson.D{
// 				{"provider-id", c.doc.ProviderId},
// 				{"scale", c.doc.DesiredScale},
// 			},
// 			Update: bson.D{{"$set", bson.D{
// 				// the scale has already been applied.
// 				{"applied", true},
// 			}}},
// 		}}, nil
// 	}
// 	if err := c.st.db().Run(buildTxn); err != nil {
// 		logger.Errorf("DesiredScaleApplied err -> %v", err)
// 		logger.Errorf("DesiredScaleApplied c.doc.DesiredScale %v", c.doc.DesiredScale)
// 		return c.doc.DesiredScale, errors.Errorf(
// 			"cannot confirm DesiredScaleApplied for application %q", c,
// 		)
// 	}
// 	return c.doc.DesiredScale, nil
// }

// ChangeScale alters the existing scale by the provided change amount, returning the new amount.
// This is used on CAAS models.
func (c *CloudService) ChangeScale(scaleChange int) (int, error) {
	newScale := c.doc.DesiredScale + scaleChange
	logger.Criticalf("ChangeScale ===========> c.doc.DesiredScale %v, scaleChange %v, newScale %v", c.doc.DesiredScale, scaleChange, newScale)
	if newScale < 0 {
		return c.doc.DesiredScale, errors.NotValidf("cannot remove more units than currently exist")
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := c.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
			alive, err := isAlive(c.st, applicationsC, c.doc.DocID)
			if err != nil {
				return nil, errors.Trace(err)
			} else if !alive {
				return nil, applicationNotAliveErr
			}
			newScale = c.doc.DesiredScale + scaleChange
			if newScale < 0 {
				return nil, errors.NotValidf("cannot remove more units than currently exist")
			}
		}
		return []txn.Op{{
			C:  cloudServicesC,
			Id: c.doc.DocID,
			Assert: bson.D{
				{"provider-id", c.doc.ProviderId},
				{"scale", c.doc.DesiredScale},
			},
			Update: bson.D{{"$set", bson.D{
				{"scale", newScale},
				// new scale has not been applied yet.
				{"applied", false},
			}}},
		}}, nil
	}
	if err := c.st.db().Run(buildTxn); err != nil {
		logger.Errorf("ChangeScale err -> %v", err)
		logger.Errorf("ChangeScale c.doc.DesiredScale %v, scaleChange %v, newScale %v", c.doc.DesiredScale, scaleChange, newScale)
		return c.doc.DesiredScale, errors.Errorf("cannot set scale for application %q to %v: %v", c, newScale, onAbort(err, applicationNotAliveErr))
	}
	c.doc.DesiredScale = newScale
	return newScale, nil
}

// SetScale sets the application's desired scale value.
// This is used on CAAS models.
func (c *CloudService) SetScale(scale int, generation int64, force bool) error {
	logger.Criticalf("SetScale c.doc.DesiredScale %v, scale %v, c.doc.Generation %v, generation %v", c.doc.DesiredScale, scale, c.doc.Generation, generation)
	if scale < 0 {
		return errors.NotValidf("application scale %d", scale)
	}
	if !c.doc.DesiredScaleApplied && (!force || scale != c.doc.DesiredScale) {
		return errors.Forbiddenf("SetScale without force before desired scale %d applied", c.doc.DesiredScale)
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := c.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
			alive, err := isAlive(c.st, applicationsC, c.doc.DocID)
			if err != nil {
				return nil, errors.Trace(err)
			} else if !alive {
				return nil, applicationNotAliveErr
			}
		}
		patchFields := bson.D{
			{"scale", scale},
		}
		if generation > c.doc.Generation {
			patchFields = append(patchFields, bson.DocElem{"generation", generation})
			if scale == c.doc.DesiredScale {
				patchFields = append(patchFields, bson.DocElem{"applied", true})
				logger.Criticalf("desired scale %d applied, so generation changed from %d to %d", c.doc.DesiredScale, c.doc.Generation, generation)
			}
		} else if generation == c.doc.Generation {
			if scale != c.doc.DesiredScale {
				return nil, errors.NewNotValid(nil, fmt.Sprintf(
					"scale changed from %d to %d for generation %d", c.doc.DesiredScale, scale, generation,
				))
			}
			logger.Warningf("no change on scale %d for generation %d", scale, generation)
			// no ops
			return nil, nil
		} else {
			if !force {
				return nil, errors.Forbiddenf(
					"application generation %d can not be reverted to %d", c.doc.Generation, generation,
				)
			}
			patchFields = append(patchFields, bson.DocElem{"applied", false})
		}
		logger.Criticalf("SetScale Update -> %+v", patchFields)
		return []txn.Op{{
			C:  cloudServicesC,
			Id: c.doc.DocID,
			Assert: bson.D{
				{"provider-id", c.doc.ProviderId},
				{"scale", c.doc.DesiredScale},
			},
			Update: bson.D{{"$set", patchFields}},
		}}, nil
	}
	if err := c.st.db().Run(buildTxn); err != nil {
		return errors.Errorf("cannot set scale for application %q to %v: %v", c, scale, onAbort(err, applicationNotAliveErr))
	}
	c.doc.DesiredScale = scale
	return nil
}

func (a *Application) removeCloudServiceOps() []txn.Op {
	ops := []txn.Op{{
		C:      cloudServicesC,
		Id:     a.globalKey(),
		Remove: true,
	}}
	return ops
}
