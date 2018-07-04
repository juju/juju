// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	humanize "github.com/dustin/go-humanize"
	"github.com/juju/errors"
	charm "gopkg.in/juju/charm.v6"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/environs/config"
)

// NewDeviceBackend creates a backend for managing device.
func NewDeviceBackend(st *State) (*deviceBackend, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &deviceBackend{
		mb: st,
		// registry:    registry,  // TODO(ycliuhw): added device registry to interact with provider
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
	// registry  devices.ProviderRegistry  TODO(ycliuhw)
	settings *StateSettings
}

// deviceConstraintsDoc contains device constraints for an entity.
type deviceConstraintsDoc struct {
	DocID       string                         `bson:"_id"`
	ModelUUID   string                         `bson:"model-uuid"`
	Constraints map[string]devices.Constraints `bson:"constraints"`
}

func createDeviceConstraintsOp(key string, cons map[string]devices.Constraints) txn.Op {
	return txn.Op{
		C:      deviceConstraintsC,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: &deviceConstraintsDoc{
			Constraints: cons,
		},
	}
}

func replaceDeviceConstraintsOp(key string, cons map[string]devices.Constraints) txn.Op {
	return txn.Op{
		C:      deviceConstraintsC,
		Id:     key,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"constraints", cons}}}},
	}
}

func removeDeviceConstraintsOp(key string) txn.Op {
	return txn.Op{
		C:      deviceConstraintsC,
		Id:     key,
		Remove: true,
	}
}
func readDeviceConstraints(mb modelBackend, key string) (map[string]devices.Constraints, error) {
	coll, closer := mb.db().GetCollection(deviceConstraintsC)
	defer closer()

	var doc deviceConstraintsDoc
	err := coll.FindId(key).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("device constraints for %q", key)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get device constraints for %q", key)
	}
	return doc.Constraints, nil
}

func validateDeviceConstraints(sb *deviceBackend, allCons map[string]devices.Constraints, charmMeta *charm.Meta) error {
	err := validateDeviceConstraintsAgainstCharm(sb, allCons, charmMeta)
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
	sb *deviceBackend,
	allCons map[string]devices.Constraints,
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
		return errors.Errorf(
			"minimum device size is %s, %s specified",
			humanize.Bytes(uint64(charmDevice.CountMin)*humanize.MByte),
			humanize.Bytes(uint64(count)*humanize.MByte),
		)
	}
	return nil
}
