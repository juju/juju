package environs

import (
	"fmt"
)

// MongoURL figures out from where to retrieve a copy of MongoDB compatible with
// the given version from the given environment. The search locations are (in order):
// - the environment specific storage
// - the public storage
// - a "well known" EC2 bucket
func MongoURL(env Environ, series, architecture string) string {
	path := MongoStoragePath(series, architecture)
	url, err := findMongo(env.Storage(), path)
	if err == nil {
		return url
	}
	url, err = findMongo(env.PublicStorage(), path)
	if err == nil {
		return url
	}
	url = fmt.Sprintf("http://juju-dist.s3.amazonaws.com/%s", path)
	return url
}

// Return the URL of a compatible MongoDB (if it exists) from the storage,
// for the given series and architecture (in vers).
func findMongo(store StorageReader, path string) (string, error) {
	names, err := store.List(path)
	if err != nil {
		return "", err
	}
	if len(names) != 1 {
		return "", &NotFoundError{fmt.Errorf("%s not found", path)}
	}
	url, err := store.URL(names[0])
	if err != nil {
		return "", err
	}
	return url, nil
}

// MongoStoragePath returns the path that is used to
// retrieve the given version of mongodb in a Storage.
func MongoStoragePath(series, architecture string) string {
	return fmt.Sprintf("tools/mongo-2.2.0-%s-%s.tgz", series, architecture)
}
