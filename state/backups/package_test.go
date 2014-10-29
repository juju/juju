// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"crypto/sha1"
	"encoding/base64"
	"io"
	"os"
	stdtesting "testing"

	"github.com/juju/utils/filestorage"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func shaSumFile(c *gc.C, file *os.File) string {
	shahash := sha1.New()
	_, err := io.Copy(shahash, file)
	c.Assert(err, gc.IsNil)
	return base64.StdEncoding.EncodeToString(shahash.Sum(nil))
}

type fakeStorage struct {
	calls   []string
	idArg   string
	metaArg filestorage.Metadata
	fileArg io.Reader

	id   string
	meta filestorage.Metadata
	file io.ReadCloser
	err  error
}

func (s *fakeStorage) check(c *gc.C, id string, meta filestorage.Metadata, file io.Reader, calls ...string) {
	c.Check(s.calls, gc.DeepEquals, calls)
	c.Check(s.idArg, gc.Equals, id)
	c.Check(s.metaArg, gc.Equals, meta)
	c.Check(s.fileArg, gc.Equals, file)
}

func (s *fakeStorage) Metadata(id string) (filestorage.Metadata, error) {
	s.calls = append(s.calls, "Metadata")
	s.idArg = id

	return s.meta, s.err
}

func (s *fakeStorage) Get(id string) (filestorage.Metadata, io.ReadCloser, error) {
	s.calls = append(s.calls, "Get")
	s.idArg = id

	return s.meta, s.file, s.err
}

func (s *fakeStorage) List() ([]filestorage.Metadata, error) {
	s.calls = append(s.calls, "List")

	return []filestorage.Metadata{s.meta}, s.err
}

func (s *fakeStorage) Add(meta filestorage.Metadata, archive io.Reader) (string, error) {
	s.calls = append(s.calls, "Add")
	s.metaArg = meta
	s.fileArg = archive
	meta.SetStored(nil)

	return s.id, s.err
}

func (s *fakeStorage) SetFile(id string, file io.Reader) error {
	s.calls = append(s.calls, "SetFile")
	s.idArg = id
	s.fileArg = file

	return s.err
}

func (s *fakeStorage) Remove(id string) error {
	s.calls = append(s.calls, "Remove")
	s.idArg = id

	return s.err
}

func (s *fakeStorage) Close() error {
	s.calls = append(s.calls, "Close")

	return s.err
}
