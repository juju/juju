// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"io"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
)

// This code was inspired by:
// https://github.com/mongodb/mongo-tools/blob/master/common/db/bson_stream.go
//
// Also see: http://bsonspec.org/spec.html.

const MaxBSONSize = 16 * 1024 * 1024

func encodeBSONSizeInto(size int32, into []byte) {
	into[0] = byte(size >> 0 & 255)
	into[1] = byte(size >> 8 & 255)
	into[2] = byte(size >> 16 & 255)
	into[3] = byte(size >> 24 & 255)
}

func decodeBSONSize(buf []byte) int32 {
	// Trust the caller to pass a buffer of at least size 4.
	return int32(
		(uint32(buf[0]) << 0) |
			(uint32(buf[1]) << 8) |
			(uint32(buf[2]) << 16) |
			(uint32(buf[3]) << 24),
	)
}

func readBSONSizeInto(in io.Reader, into []byte) (int32, error) {
	_, err := io.ReadAtLeast(in, into[0:4], 4)
	if err != nil {
		return -1, errors.Trace(err)
	}
	return decodeBSONSize(into), nil
}

type bsonIterator struct {
	in        io.Reader
	buf       []byte
	remainder int32
	err       error
}

func IterBSON(in io.Reader) (*bsonIterator, int, error) {
	buf := make([]byte, MaxBSONSize)

	remainder, err := readBSONSizeInto(in, buf)
	if err != nil {
		return nil, -1, errors.Trace(err)
	}

	bit := bsonIterator{
		in:        in,
		buf:       buf,
		remainder: remainder,
	}
	return &bit, int(remainder), nil
}

func (bit *bsonIterator) Err() error {
	return bit.err
}

func (bit *bsonIterator) NextRaw(into []byte, size *int) bool {
	var err error

	*size, err = bit.readSize(into)
	if err != nil {
		if *size < 0 && errors.Cause(err) == io.EOF {
			err = nil
		}
		bit.err = errors.Trace(err)
		return false
	}

	if err := bit.readData(into, *size); err != nil {
		bit.err = errors.Trace(err)
		return false
	}

	bit.err = nil
	return true
}

func (bit *bsonIterator) Next(into *bson.Raw) bool {
	var size int
	if !bit.NextRaw(bit.buf, &size) {
		return false
	}
	if err := bson.Unmarshal(bit.buf[0:size], into); err != nil {
		bit.err = errors.Trace(err)
		return false
	}
	return true
}

func (bit *bsonIterator) readSize(into []byte) (int, error) {
	if bit.remainder < 4 {
		if bit.remainder != 0 {
			return -1, errors.Errorf("invalid BSON entry")
		} else {
			return -1, io.EOF
		}
	}
	if len(into) < 4 {
		return -1, errors.Errorf("buffer too small: need at least size 4, got %d", len(into))
	}
	size, err := readBSONSizeInto(bit.in, into)
	if err != nil {
		return -1, errors.Trace(err)
	}
	bit.remainder -= 4
	if size > bit.remainder {
		return -1, errors.Errorf("entry exceeds remaining size (%d): %d", bit.remainder, size)
	}
	return int(size), nil
}

func (bit *bsonIterator) readData(into []byte, size int) error {
	if len(into) < size {
		return errors.Errorf("buffer too small: need at least size %d, got %d", size, len(into))
	}

	n, err := io.ReadAtLeast(bit.in, into[4:size], size-4)
	bit.remainder -= int32(n)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}
