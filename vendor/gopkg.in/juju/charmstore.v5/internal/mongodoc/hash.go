// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongodoc // import "gopkg.in/juju/charmstore.v5/internal/mongodoc"

import (
	"crypto/sha512"
	"encoding/hex"

	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2/bson"
)

// Hashes holds a slice of hash objects
// that will encode to MongoDB in a space-efficient
// way. Each of the strings must be a hex-encoded
// SHA384 checksum as produced by blobstore.NewHash.
type Hashes []string

// SetBSON implements bson.Setter by unmarshaling
// the document from a byte slice.
func (h *Hashes) SetBSON(raw bson.Raw) error {
	var slice []byte
	if err := raw.Unmarshal(&slice); err != nil {
		return errgo.Mask(err)
	}
	if len(slice)%sha512.Size384 != 0 {
		return errgo.Newf("length %d not a multiple of hash size", len(slice))
	}
	hashes := make([]string, len(slice)/sha512.Size384)
	for i := range hashes {
		hashes[i] = hex.EncodeToString(slice[i*sha512.Size384 : (i+1)*sha512.Size384])
	}
	*h = hashes
	return nil
}

// GetBSON implements bson.Getter by marshaling
// the hash slice to a contiguous byte slice.
func (hs Hashes) GetBSON() (interface{}, error) {
	slice := make([]byte, len(hs)*sha512.Size384)
	for i, h := range hs {
		if len(h) != sha512.Size384*2 {
			return nil, errgo.Newf("invalid hash length of %q", h)
		}
		_, err := hex.Decode(slice[i*sha512.Size384:(i+1)*sha512.Size384], []byte(h))
		if err != nil {
			return nil, errgo.Notef(err, "invalid hash encoding %q", h)
		}
	}
	return slice, nil
}
