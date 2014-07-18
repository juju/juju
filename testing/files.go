// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
)

//---------------------------
// FakeFile

// FakeFile is a fake file that implements io.Reader, io.Writer, and
// io.Closer.
//
// For now only errors are supported for the three methods.  Each of
// them will fail if the corresponding values are set on the struct.
type FakeFile struct {
	ReadSize   int
	ReadError  string
	WriteSize  int
	WriteError string
	CloseError string
}

// Other possible additions:
// + add a bytes.Buffer field to store the data read/written
// + support other os.File methods (like Seek() and Stat())

func (rw *FakeFile) Read([]byte) (int, error) {
	var err error
	if rw.ReadError != "" {
		err = fmt.Errorf(rw.ReadError)
	}
	return rw.ReadSize, err
}

func (rw *FakeFile) Write([]byte) (int, error) {
	var err error
	if rw.WriteError != "" {
		err = fmt.Errorf(rw.WriteError)
	}
	return rw.WriteSize, err
}

func (rw *FakeFile) Close() error {
	var err error
	if rw.CloseError != "" {
		err = fmt.Errorf(rw.CloseError)
	}
	return err
}
