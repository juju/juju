// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"bytes"
	"fmt"
	"io"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/resources"
)

// dockerMetadataStorage implements DockerMetadataStorage
type dockerMetadataStorage struct {
	st *State
}

type dockerMetadataDoc struct {
	// Id holds the resource ID
	Id string `bson:"_id"`

	// Registry holds the URI/URL of the registry where the image is stored.
	Registry string `bson:"registry"`

	// Image holds the image path part of the whole url, to be used with the registry URI.
	Image string `bson:"image"`

	// Username holds the password string for a non-private image.
	Username string `bson:"username"`

	// Password holds the password string for a non-private image.
	Password string `bson:"password"`
}

// DockerMetadataStorage provides the interface for storing Docker resource-type data
type DockerMetadataStorage interface {
	Save(resourceID string, drInfo resources.DockerImageDetails) error
	Remove(resourceID string) error
	Get(resourceID string) (io.ReadCloser, int64, error)
}

// NewDockerMetadataStorage returns a dockerMetadataStorage for persisting Docker resources.
func NewDockerMetadataStorage(st *State) DockerMetadataStorage {
	return &dockerMetadataStorage{
		st: st,
	}
}

// Save creates a new record the a Docker resource.
func (dr *dockerMetadataStorage) Save(resourceID string, drInfo resources.DockerImageDetails) error {

	registry, image, err := resources.SplitRegistryPath(drInfo.RegistryPath)
	if err != nil {
		return errors.Annotate(err, "failed to extract repo and image")
	}
	doc := dockerMetadataDoc{
		Id:       resourceID,
		Registry: registry,
		Image:    image,
		Username: drInfo.Username,
		Password: drInfo.Password,
	}

	buildTxn := func(int) ([]txn.Op, error) {
		existing, err := dr.get(resourceID)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Annotate(err, "failed to check for existing resource")

		}
		if !errors.IsNotFound(err) {
			return []txn.Op{{
				C:      dockerResourcesC,
				Id:     existing.Id,
				Assert: txn.DocExists,
				Update: bson.D{
					{"$set",
						bson.D{
							{"registry", doc.Registry},
							{"image", doc.Image},
							{"username", doc.Username},
							{"password", doc.Password},
						},
					},
				},
			}}, nil
		}

		return []txn.Op{{
			C:      dockerResourcesC,
			Id:     doc.Id,
			Assert: txn.DocMissing,
			Insert: doc,
		}}, nil
	}

	err = dr.st.db().Run(buildTxn)
	return errors.Annotate(err, "failed to store Docker resource")
}

// Remove removes the Docker resource with the provided ID.
func (dr *dockerMetadataStorage) Remove(resourceID string) error {
	ops := []txn.Op{{
		C:      dockerResourcesC,
		Id:     resourceID,
		Remove: true,
	}}
	err := dr.st.db().RunTransaction(ops)
	return errors.Annotate(err, "failed to remove Docker resource")
}

// Get retrieves the requested stored Docker resource.
func (dr *dockerMetadataStorage) Get(resourceID string) (io.ReadCloser, int64, error) {
	doc, err := dr.get(resourceID)
	if err != nil {
		return nil, -1, errors.Trace(err)
	}
	separator := ""
	if doc.Registry != "" {
		separator = "/"
	}
	data, err := yaml.Marshal(
		resources.DockerImageDetails{
			RegistryPath: fmt.Sprintf("%s%s%s", doc.Registry, separator, doc.Image),
			Username:     doc.Username,
			Password:     doc.Password,
		})
	if err != nil {
		return nil, -1, errors.Trace(err)
	}
	infoReader := bytes.NewReader(data)
	length := infoReader.Len()
	return &dockerResourceReadCloser{infoReader}, int64(length), nil
}

func (dr *dockerMetadataStorage) get(resourceID string) (*dockerMetadataDoc, error) {
	coll, closer := dr.st.db().GetCollection(dockerResourcesC)
	defer closer()

	var doc dockerMetadataDoc
	err := coll.FindId(resourceID).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("Docker resource with ID: %s", resourceID)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

type dockerResourceReadCloser struct {
	io.ReadSeeker
}

func (drrc *dockerResourceReadCloser) Close() error {
	return nil
}
