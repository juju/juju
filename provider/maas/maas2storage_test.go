// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type maas2StorageSuite struct {
	maas2Suite
}

var _ = gc.Suite(&maas2StorageSuite{})

func (s *maas2StorageSuite) makeStorage(c *gc.C, controller gomaasapi.Controller) *maas2Storage {
	storage, ok := NewStorage(s.makeEnviron(c, controller)).(*maas2Storage)
	c.Assert(ok, jc.IsTrue)
	ecfg := storage.environ.ecfg()
	ecfg.attrs["maas-agent-name"] = "prefix"
	return storage
}

func getController(c *gc.C, storage *maas2Storage) *fakeController {
	result, ok := storage.maasController.(*fakeController)
	c.Assert(ok, jc.IsTrue)
	return result
}

func (s *maas2StorageSuite) TestGetNoSuchFile(c *gc.C) {
	storage := s.makeStorage(c, &fakeController{
		filesError: errors.New("This file no existence"),
	})
	_, err := storage.Get("grasshopper.avi")
	c.Assert(err, gc.ErrorMatches, "This file no existence")
}

func (s *maas2StorageSuite) TestGetReadFails(c *gc.C) {
	storage := s.makeStorage(c, &fakeController{
		files: []gomaasapi.File{
			&fakeFile{
				name:  "prefix-grasshopper.avi",
				error: errors.New("read error"),
			},
		},
	})
	_, err := storage.Get("grasshopper.avi")
	c.Assert(err, gc.ErrorMatches, "read error")
}

func (s *maas2StorageSuite) TestGetSuccess(c *gc.C) {
	controller := &fakeController{
		files: []gomaasapi.File{
			&fakeFile{
				name:     "prefix-grasshopper.avi",
				contents: []byte("The man in the high castle"),
			},
		},
	}
	storage := s.makeStorage(c, controller)
	reader, err := storage.Get("grasshopper.avi")
	c.Assert(err, jc.ErrorIsNil)
	defer reader.Close()
	result, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []byte("The man in the high castle"))
	c.Assert(controller.getFileFilename, gc.Equals, "prefix-grasshopper.avi")
}

func (s *maas2StorageSuite) TestListError(c *gc.C) {
	storage := s.makeStorage(c, &fakeController{
		filesError: errors.New("couldn't list files"),
	})
	_, err := storage.List("american-territories")
	c.Assert(err, gc.ErrorMatches, "couldn't list files")
}

func (s *maas2StorageSuite) TestListSuccess(c *gc.C) {
	controller := &fakeController{
		files: []gomaasapi.File{
			&fakeFile{name: "prefix-julianna"},
			&fakeFile{name: "prefix-frank"},
		},
	}
	storage := s.makeStorage(c, controller)
	result, err := storage.List("grasshopper")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []string{"frank", "julianna"})
	c.Assert(controller.filesPrefix, gc.Equals, "prefix-grasshopper")
}

func (s *maas2StorageSuite) TestURLError(c *gc.C) {
	storage := s.makeStorage(c, &fakeController{
		filesError: errors.New("no such file"),
	})
	_, err := storage.URL("grasshopper.avi")
	c.Assert(err, gc.ErrorMatches, "no such file")
}

func (s *maas2StorageSuite) TestURLSuccess(c *gc.C) {
	controller := &fakeController{
		files: []gomaasapi.File{
			&fakeFile{name: "prefix-grasshopper.avi", url: "heavy lies"},
		},
	}
	storage := s.makeStorage(c, controller)
	result, err := storage.URL("grasshopper.avi")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "heavy lies")
	c.Assert(controller.getFileFilename, gc.Equals, "prefix-grasshopper.avi")
}

func (s *maas2StorageSuite) TestPut(c *gc.C) {
	controller := &fakeController{
		filesError: errors.New("oh no!"),
	}
	storage := s.makeStorage(c, controller)
	reader := bytes.NewReader([]byte{})
	err := storage.Put("riff", reader, 10)
	c.Assert(err, gc.ErrorMatches, "oh no!")
	c.Assert(controller.addFileArgs, jc.DeepEquals, gomaasapi.AddFileArgs{
		Filename: "prefix-riff",
		Reader:   reader,
		Length:   10,
	})
}

func (s *maas2StorageSuite) TestRemoveNoSuchFile(c *gc.C) {
	controller := &fakeController{
		filesError: errors.New("oh no!"),
	}
	storage := s.makeStorage(c, controller)
	err := storage.Remove("FIOS")
	c.Assert(err, gc.ErrorMatches, "oh no!")
}

func (s *maas2StorageSuite) TestRemoveErrorFromDelete(c *gc.C) {
	controller := &fakeController{
		files: []gomaasapi.File{
			&fakeFile{name: "prefix-FIOS", error: errors.New("protected")},
		},
	}
	storage := s.makeStorage(c, controller)
	err := storage.Remove("FIOS")
	c.Assert(err, gc.ErrorMatches, "protected")
	c.Assert(controller.getFileFilename, gc.Equals, "prefix-FIOS")
}

func (s *maas2StorageSuite) TestRemoveAll(c *gc.C) {
	controller := &fakeController{
		files: []gomaasapi.File{
			&fakeFile{name: "prefix-zack"},
			&fakeFile{name: "prefix-kevin", error: errors.New("oops")},
			&fakeFile{name: "prefix-jim"},
			&fakeFile{name: "prefix-riff"},
		},
	}
	storage := s.makeStorage(c, controller)
	err := storage.RemoveAll()
	c.Assert(err, gc.ErrorMatches, "cannot delete all provider state: oops")
	c.Assert(controller.filesPrefix, gc.Equals, "prefix-")

	deleteds := make([]bool, 4)
	for i, file := range controller.files {
		deleteds[i] = file.(*fakeFile).deleted
	}
	c.Assert(deleteds, jc.DeepEquals, []bool{true, true, true, true})
}
