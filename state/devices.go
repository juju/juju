// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/environs/config"
)

type DeviceType string

// DeviceConstraints describes a set of device constraints.
type DeviceConstraints struct {

	// Type is the device type or device-class.
	// currently supported types are
	// - gpu
	// - nvidia.com/gpu
	// - amd.com/gpu
	Type DeviceType `bson:"type"`

	// Count is the number of devices that the user has asked for - count min and max are the
	// number of devices the charm requires.
	Count int64 `bson:"count"`

	// Attributes is a collection of key value pairs device related (node affinity labels/tags etc.).
	Attributes map[string]string `bson:"attributes"`
}

// NewDeviceBackend creates a backend for managing device.
func NewDeviceBackend(st *State) (*deviceBackend, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &deviceBackend{
		mb:          st,
		settings:    NewStateSettings(st),
		modelType:   m.Type(),
		config:      m.ModelConfig,
		application: st.Application,
		unit:        st.Unit,
		machine:     st.Machine,
	}, nil
}

type deviceBackend struct {
	mb          modelBackend
	config      func() (*config.Config, error)
	application func(string) (*Application, error)
	unit        func(string) (*Unit, error)
	machine     func(string) (*Machine, error)

	modelType ModelType
	settings  *StateSettings
}

// deviceConstraintsDoc contains device constraints for an entity.
type deviceConstraintsDoc struct {
	DocID       string                       `bson:"_id"`
	Constraints map[string]DeviceConstraints `bson:"constraints"`
}

func createDeviceConstraintsOp(id string, cons map[string]DeviceConstraints) txn.Op {
	return txn.Op{
		C:      deviceConstraintsC,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: &deviceConstraintsDoc{
			Constraints: cons,
		},
	}
}

func removeDeviceConstraintsOp(id string) txn.Op {
	return txn.Op{
		C:      deviceConstraintsC,
		Id:     id,
		Remove: true,
	}
}

// DeviceConstraints returns the device constraints for the specified application.
func (db *deviceBackend) DeviceConstraints(id string) (map[string]DeviceConstraints, error) {
	devices, err := readDeviceConstraints(db.mb, id)
	if err == nil {
		return devices, nil
	} else if errors.IsNotFound(err) {
		return map[string]DeviceConstraints{}, nil
	}
	return nil, err
}

func readDeviceConstraints(mb modelBackend, id string) (map[string]DeviceConstraints, error) {
	coll, closer := mb.db().GetCollection(deviceConstraintsC)
	defer closer()

	var doc deviceConstraintsDoc
	err := coll.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("device constraints for %q", id)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get device constraints for %q", id)
	}
	return doc.Constraints, nil
}

func validateDeviceConstraints(db *deviceBackend, allCons map[string]DeviceConstraints, charmMeta *charm.Meta) error {
	err := validateDeviceConstraintsAgainstCharm(db, allCons, charmMeta)
	if err != nil {
		return errors.Trace(err)
	}
	// Ensure all devices have constraints specified. Defaults should have
	// been set by this point, if the user didn't specify constraints.
	for name, charmDevice := range charmMeta.Devices {
		if _, ok := allCons[name]; !ok && charmDevice.CountMin > 0 {
			return errors.Errorf("no constraints specified for device %q", name)
		}
	}
	return nil
}

func validateDeviceConstraintsAgainstCharm(
	db *deviceBackend,
	allCons map[string]DeviceConstraints,
	charmMeta *charm.Meta,
) error {
	for name, cons := range allCons {
		charmDevice, ok := charmMeta.Devices[name]
		if !ok {
			return errors.Errorf("charm %q has no device called %q", charmMeta.Name, name)
		}
		if err := validateCharmDeviceCount(charmDevice, cons.Count); err != nil {
			return errors.Annotatef(err, "charm %q device %q", charmMeta.Name, name)
		}

	}
	return nil
}

func validateCharmDeviceCount(charmDevice charm.Device, count int64) error {
	if charmDevice.CountMin > 0 && count < charmDevice.CountMin {
		return errors.Errorf("minimum device size is %d, %d specified", charmDevice.CountMin, count)
	}
	return nil
}
