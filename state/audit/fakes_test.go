// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit_test

import (
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/mongo"
)

type fakeWriteCollection struct {
	fakeCollection

	insert func(...interface{}) error
}

// All other methods act as documented for *mgo.Collection.
func (f fakeWriteCollection) Insert(docs ...interface{}) error {
	if f.insert != nil {
		return f.insert(docs...)
	}
	return nil
}

func (f fakeWriteCollection) Underlying() *mgo.Collection { return nil }

func (f fakeWriteCollection) Upsert(selector interface{}, update interface{}) (info *mgo.ChangeInfo, err error) {
	return nil, nil
}

func (f fakeWriteCollection) UpsertId(id interface{}, update interface{}) (info *mgo.ChangeInfo, err error) {
	return nil, nil
}

func (f fakeWriteCollection) Update(selector interface{}, update interface{}) error {
	return nil
}

func (f fakeWriteCollection) UpdateId(id interface{}, update interface{}) error {
	return nil
}

func (f fakeWriteCollection) Remove(sel interface{}) error {
	return nil
}

func (f fakeWriteCollection) RemoveId(id interface{}) error {
	return nil
}

func (f fakeWriteCollection) RemoveAll(sel interface{}) (*mgo.ChangeInfo, error) {
	return nil, nil
}

type fakeCollection struct {
	FakeName string

	writeable func() mongo.WriteCollection
}

func (f fakeCollection) Name() string {
	return f.FakeName
}

func (f fakeCollection) Count() (int, error) {
	return 0, nil
}

func (f fakeCollection) Find(query interface{}) mongo.Query {
	return nil
}

func (f fakeCollection) FindId(id interface{}) mongo.Query {
	return nil
}

func (f fakeCollection) Writeable() mongo.WriteCollection {
	if f.writeable != nil {
		return f.writeable()
	}
	return nil
}
