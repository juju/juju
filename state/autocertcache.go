// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"golang.org/x/crypto/acme/autocert"

	"github.com/juju/juju/internal/mongo"
)

// AutocertCache returns an implementation
// of autocert.Cache backed by the state.
func (st *State) AutocertCache() autocert.Cache {
	return autocertCache{st}
}

type autocertCache struct {
	st *State
}

type autocertCacheDoc struct {
	Name string `bson:"_id"`
	Data []byte `bson:"data"`
}

// Put implements autocert.Cache.Put.
func (cache autocertCache) Put(ctx context.Context, name string, data []byte) error {
	coll, closeColl := cache.coll()
	defer closeColl()
	_, err := coll.UpsertId(name, autocertCacheDoc{
		Name: name,
		Data: data,
	})
	if err != nil {
		return errors.Annotatef(err, "cannot store autocert key %q", name)
	}
	return nil
}

// Get implements autocert.Cache.Get.
func (cache autocertCache) Get(ctx context.Context, name string) ([]byte, error) {
	coll, closeColl := cache.coll()
	defer closeColl()
	var doc autocertCacheDoc
	err := coll.FindId(name).One(&doc)
	if err == nil {
		return doc.Data, nil
	}
	if errors.Cause(err) == mgo.ErrNotFound {
		return nil, autocert.ErrCacheMiss
	}
	return nil, errors.Annotatef(err, "cannot get autocert key %q", name)
}

// Delete implements autocert.Cache.Delete.
func (cache autocertCache) Delete(ctx context.Context, name string) error {
	coll, closeColl := cache.coll()
	defer closeColl()
	err := coll.RemoveId(name)
	if err == nil || errors.Cause(err) == mgo.ErrNotFound {
		return nil
	}
	return errors.Annotatef(err, "cannot delete autocert key %q", name)
}

func (cache autocertCache) coll() (mongo.WriteCollection, func()) {
	coll, closer := cache.st.db().GetCollection(autocertCacheC)
	return coll.Writeable(), closer
}
