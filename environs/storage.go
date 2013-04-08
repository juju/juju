package environs

import (
	"fmt"
	"io"
	"launchpad.net/juju-core/environs/storage"
)

// EmptyStorage holds a storage.Reader object that contains no files and
// offers no URLs.
var EmptyStorage storage.Reader = emptyStorage{}

type emptyStorage struct{}

func (s emptyStorage) Get(name string) (io.ReadCloser, error) {
	return nil, &NotFoundError{fmt.Errorf("file %q not found", name)}
}

func (s emptyStorage) URL(name string) (string, error) {
	return "", fmt.Errorf("file %q not found", name)
}

func (s emptyStorage) List(prefix string) ([]string, error) {
	return nil, nil
}
