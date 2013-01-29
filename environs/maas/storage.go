package maas

import (
	"io"
	"launchpad.net/juju-core/environs"
)

type maasStorage struct{}

var _ environs.Storage = (*maasStorage)(nil)

func (*maasStorage) Get(name string) (io.ReadCloser, error) {
	panic("Not implemented.")
}

func (*maasStorage) List(prefix string) ([]string, error) {
	panic("Not implemented.")
}

func (*maasStorage) URL(name string) (string, error) {
	panic("Not implemented.")
}

func (*maasStorage) Put(name string, r io.Reader, length int64) error {
	panic("Not implemented.")
}

func (*maasStorage) Remove(name string) error {
	panic("Not implemented.")
}
