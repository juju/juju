// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/txn"
	"github.com/juju/names/v4"

	"github.com/juju/juju/v2/core/leadership"
)

type containerSpecDoc struct {
	// Id holds container spec document key.
	// It is the global key of the application represented
	// by this container.
	Id string `bson:"_id"`

	Spec string `bson:"spec"`
	// RawSpec is the raw format of k8s spec.
	RawSpec string `bson:"raw-spec"`

	UpgradeCounter int `bson:"upgrade-counter"`
}

// SetPodSpec sets the pod spec for the given application tag while making sure
// that the caller is the leader by validating the provided token. For cases
// where leadership checks are not important (e.g. migrations), a nil Token can
// be provided to bypass the leadership checks.
//
// An error will be returned if the specified application is not alive or the
// leadership check fails.
func (m *CAASModel) SetPodSpec(token leadership.Token, appTag names.ApplicationTag, spec *string) error {
	modelOp := m.SetPodSpecOperation(token, appTag, spec)
	return m.st.ApplyOperation(modelOp)
}

// SetPodSpecOperation returns a ModelOperation for updating a PodSpec. For
// cases where leadership checks are not important (e.g. migrations), a nil
// Token can be provided to bypass the leadership checks.
func (m *CAASModel) SetPodSpecOperation(token leadership.Token, appTag names.ApplicationTag, spec *string) ModelOperation {
	return newSetPodSpecOperation(m, token, appTag, spec)
}

// SetRawK8sSpecOperation returns a ModelOperation for updating a raw k8s spec. For
// cases where leadership checks are not important (e.g. migrations), a nil
// Token can be provided to bypass the leadership checks.
func (m *CAASModel) SetRawK8sSpecOperation(token leadership.Token, appTag names.ApplicationTag, spec *string) ModelOperation {
	return newSetRawK8sSpecOperation(m, token, appTag, spec)
}

// RawK8sSpec returns the raw k8s spec for the given application tag.
func (m *CAASModel) RawK8sSpec(appTag names.ApplicationTag) (string, error) {
	info, err := m.podInfo(appTag)
	if err != nil {
		return "", errors.Trace(err)
	}
	return info.RawSpec, nil
}

// PodSpec returns the pod spec for the given application tag.
func (m *CAASModel) PodSpec(appTag names.ApplicationTag) (string, error) {
	info, err := m.podInfo(appTag)
	if err != nil {
		return "", errors.Trace(err)
	}
	return info.Spec, nil
}

func (m *CAASModel) podInfo(appTag names.ApplicationTag) (*containerSpecDoc, error) {
	var doc containerSpecDoc
	if err := readPodInfo(m.mb.db(), appTag.Id(), &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func readPodInfo(db Database, appName string, doc interface{}) error {
	coll, cleanup := db.GetCollection(podSpecsC)
	defer cleanup()
	if err := coll.FindId(applicationGlobalKey(appName)).One(doc); err != nil {
		if err == mgo.ErrNotFound {
			return errors.NotFoundf("k8s spec for application %s", appName)
		}
		return errors.Trace(err)
	}
	return nil
}

func removePodSpecOp(appTag names.ApplicationTag) txn.Op {
	return txn.Op{
		C:      podSpecsC,
		Id:     applicationGlobalKey(appTag.Id()),
		Remove: true,
	}
}
