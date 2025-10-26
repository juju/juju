// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filetesting

import (
	"bytes"
	"hash"
	"io"
	"os"
	"time"
)

type StubReader struct {
	Stub any

	ReturnRead io.Reader
}

func NewStubReader(stub any, content string) io.Reader {
	return &StubReader{}
}

func (s *StubReader) Read(data []byte) (int, error) {
	panic("unimplemented")
}

type StubWriter struct {
	Stub any

	ReturnWrite io.Writer
}

func NewStubWriter(stub any) (io.Writer, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	s := &StubWriter{
		Stub:        stub,
		ReturnWrite: buf,
	}
	return s, buf
}

func (s *StubWriter) Write(data []byte) (int, error) {
	panic("unimplemented")
}

type StubSeeker struct {
	Stub any

	ReturnSeek int64
}

func (s *StubSeeker) Seek(offset int64, whence int) (int64, error) {
	panic("unimplemented")
}

type StubCloser struct {
	Stub any
}

func (s *StubCloser) Close() error {
	panic("unimplemented")
}

type StubFile struct {
	io.Reader
	io.Writer
	io.Seeker
	io.Closer

	Stub any
	Info StubFileInfo
}

func NewStubFile(stub any, raw io.ReadWriter) *StubFile {
	return &StubFile{
		Reader: &StubReader{Stub: stub, ReturnRead: raw},
		Writer: &StubWriter{Stub: stub, ReturnWrite: raw},
		Seeker: &StubSeeker{Stub: stub},
		Closer: &StubCloser{Stub: stub},
		Stub:   stub,
	}
}

func (s *StubFile) Name() string {
	panic("unimplemented")
}

func (s *StubFile) Stat() (os.FileInfo, error) {
	panic("unimplemented")
}

func (s *StubFile) Sync() error {
	panic("unimplemented")
}

func (s *StubFile) Truncate(size int64) error {
	panic("unimplemented")
}

type FileInfo struct {
	Name    string
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
}

var _ os.FileInfo = (*StubFileInfo)(nil)

type StubFileInfo struct {
	Stub any

	Info      FileInfo
	ReturnSys interface{}
}

func NewStubFileInfo(stub any, name, content string) *StubFileInfo {
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
	panic("unimplemented")
}

func (s StubFileInfo) Size() int64 {
	panic("unimplemented")
}

func (s StubFileInfo) Mode() os.FileMode {
	panic("unimplemented")
}

func (s StubFileInfo) ModTime() time.Time {
	panic("unimplemented")
}

func (s StubFileInfo) IsDir() bool {
	panic("unimplemented")
}

func (s StubFileInfo) Sys() interface{} {
	panic("unimplemented")
}

var _ hash.Hash = (*StubHash)(nil)

type StubHash struct {
	io.Writer

	Stub            any
	ReturnSum       []byte
	ReturnSize      int
	ReturnBlockSize int
}

func NewStubHash(stub any, raw io.Writer) *StubHash {
	return &StubHash{
		Writer: &StubWriter{Stub: stub, ReturnWrite: raw},
		Stub:   stub,
	}
}

func (s *StubHash) Sum(b []byte) []byte {
	panic("unimplemented")
}

func (s *StubHash) Reset() {
	panic("unimplemented")
}

func (s *StubHash) Size() int {
	panic("unimplemented")
}

func (s *StubHash) BlockSize() int {
	panic("unimplemented")
}
