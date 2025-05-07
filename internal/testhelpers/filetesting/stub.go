// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filetesting

import (
	"bytes"
	"hash"
	"io"
	"os"
	"strings"
	"time"

	"github.com/juju/errors"

	testing "github.com/juju/juju/internal/testhelpers"
)

type StubReader struct {
	Stub *testing.Stub

	ReturnRead io.Reader
}

func NewStubReader(stub *testing.Stub, content string) io.Reader {
	return &StubReader{
		Stub:       stub,
		ReturnRead: strings.NewReader(content),
	}
}

func (s *StubReader) Read(data []byte) (int, error) {
	s.Stub.AddCall("Read", data)
	if err := s.Stub.NextErr(); err != nil {
		return 0, errors.Trace(err)
	}

	if s.ReturnRead == nil {
		return 0, nil
	}
	return s.ReturnRead.Read(data)
}

type StubWriter struct {
	Stub *testing.Stub

	ReturnWrite io.Writer
}

func NewStubWriter(stub *testing.Stub) (io.Writer, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	s := &StubWriter{
		Stub:        stub,
		ReturnWrite: buf,
	}
	return s, buf
}

func (s *StubWriter) Write(data []byte) (int, error) {
	s.Stub.AddCall("Write", data)
	if err := s.Stub.NextErr(); err != nil {
		return 0, errors.Trace(err)
	}

	if s.ReturnWrite == nil {
		return 0, nil
	}
	return s.ReturnWrite.Write(data)
}

type StubSeeker struct {
	Stub *testing.Stub

	ReturnSeek int64
}

func (s *StubSeeker) Seek(offset int64, whence int) (int64, error) {
	s.Stub.AddCall("Seek", offset, whence)
	if err := s.Stub.NextErr(); err != nil {
		return 0, errors.Trace(err)
	}

	return s.ReturnSeek, nil
}

type StubCloser struct {
	Stub *testing.Stub
}

func (s *StubCloser) Close() error {
	s.Stub.AddCall("Close")
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type StubFile struct {
	io.Reader
	io.Writer
	io.Seeker
	io.Closer

	Stub *testing.Stub
	Info StubFileInfo
}

func NewStubFile(stub *testing.Stub, raw io.ReadWriter) *StubFile {
	return &StubFile{
		Reader: &StubReader{Stub: stub, ReturnRead: raw},
		Writer: &StubWriter{Stub: stub, ReturnWrite: raw},
		Seeker: &StubSeeker{Stub: stub},
		Closer: &StubCloser{Stub: stub},
		Stub:   stub,
	}
}

func (s *StubFile) Name() string {
	s.Stub.AddCall("Name")
	s.Stub.NextErr() // Pop one off.

	return s.Info.Info.Name
}

func (s *StubFile) Stat() (os.FileInfo, error) {
	s.Stub.AddCall("Stat")
	if err := s.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return &s.Info, nil
}

func (s *StubFile) Sync() error {
	s.Stub.AddCall("Sync")
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *StubFile) Truncate(size int64) error {
	s.Stub.AddCall("Truncate", size)
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type FileInfo struct {
	Name    string
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
}

var _ os.FileInfo = (*StubFileInfo)(nil)

type StubFileInfo struct {
	Stub *testing.Stub

	Info      FileInfo
	ReturnSys interface{}
}

func NewStubFileInfo(stub *testing.Stub, name, content string) *StubFileInfo {
	return &StubFileInfo{
		Stub: stub,
		Info: FileInfo{
			Name:    name,
			Size:    int64(len(content)),
			Mode:    0644,
			ModTime: time.Now(),
		},
	}
}

func (s StubFileInfo) Name() string {
	s.Stub.AddCall("Name")
	s.Stub.NextErr() // Pop one off.

	return s.Info.Name
}

func (s StubFileInfo) Size() int64 {
	s.Stub.AddCall("Size")
	s.Stub.NextErr() // Pop one off.

	return s.Info.Size
}

func (s StubFileInfo) Mode() os.FileMode {
	s.Stub.AddCall("Mode")
	s.Stub.NextErr() // Pop one off.

	return s.Info.Mode
}

func (s StubFileInfo) ModTime() time.Time {
	s.Stub.AddCall("ModTime")
	s.Stub.NextErr() // Pop one off.

	return s.Info.ModTime
}

func (s StubFileInfo) IsDir() bool {
	s.Stub.AddCall("IsDir")
	s.Stub.NextErr() // Pop one off.

	return s.Info.Mode.IsDir()
}

func (s StubFileInfo) Sys() interface{} {
	s.Stub.AddCall("Sys")
	s.Stub.NextErr() // Pop one off.

	return s.ReturnSys
}

var _ hash.Hash = (*StubHash)(nil)

type StubHash struct {
	io.Writer

	Stub            *testing.Stub
	ReturnSum       []byte
	ReturnSize      int
	ReturnBlockSize int
}

func NewStubHash(stub *testing.Stub, raw io.Writer) *StubHash {
	return &StubHash{
		Writer: &StubWriter{Stub: stub, ReturnWrite: raw},
		Stub:   stub,
	}
}

func (s *StubHash) Sum(b []byte) []byte {
	s.Stub.AddCall("Sum", b)
	s.Stub.NextErr() // Pop one off.

	return s.ReturnSum
}

func (s *StubHash) Reset() {
	s.Stub.AddCall("Reset")
	s.Stub.NextErr() // Pop one off.
}

func (s *StubHash) Size() int {
	s.Stub.AddCall("Size")
	s.Stub.NextErr() // Pop one off.

	return s.ReturnSize
}

func (s *StubHash) BlockSize() int {
	s.Stub.AddCall("BlockSize")
	s.Stub.NextErr() // Pop one off.

	return s.ReturnBlockSize
}
