// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blobstore // import "gopkg.in/juju/charmstore.v5-unstable/internal/blobstore"

import (
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"strconv"

	"github.com/juju/blobstore"
	"github.com/juju/errors"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"
)

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

// ContentChallengeError holds a proof-of-content
// challenge produced by a blobstore.
type ContentChallengeError struct {
	Req ContentChallenge
}

func (e *ContentChallengeError) Error() string {
	return "cannot upload because proof of content ownership is required"
}

// ContentChallenge holds a proof-of-content challenge
// produced by a blobstore. A client can satisfy the request
// by producing a ContentChallengeResponse containing
// the same request id and a hash of RangeLength bytes
// of the content starting at RangeStart.
type ContentChallenge struct {
	RequestId   string
	RangeStart  int64
	RangeLength int64
}

// ContentChallengeResponse holds a response to a ContentChallenge.
type ContentChallengeResponse struct {
	RequestId string
	Hash      string
}

// NewHash is used to calculate checksums for the blob store.
func NewHash() hash.Hash {
	return sha512.New384()
}

// NewContentChallengeResponse can be used by a client to respond to a content
// challenge. The returned value should be passed to BlobStorage.Put
// when the client retries the request.
func NewContentChallengeResponse(chal *ContentChallenge, r io.ReadSeeker) (*ContentChallengeResponse, error) {
	_, err := r.Seek(chal.RangeStart, 0)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	hash := NewHash()
	nw, err := io.CopyN(hash, r, chal.RangeLength)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if nw != chal.RangeLength {
		return nil, errgo.Newf("content is not long enough")
	}
	return &ContentChallengeResponse{
		RequestId: chal.RequestId,
		Hash:      fmt.Sprintf("%x", hash.Sum(nil)),
	}, nil
}

// Store stores data blobs in mongodb, de-duplicating by
// blob hash.
type Store struct {
	mstore blobstore.ManagedStorage
}

// New returns a new blob store that writes to the given database,
// prefixing its collections with the given prefix.
func New(db *mgo.Database, prefix string) *Store {
	rs := blobstore.NewGridFS(db.Name, prefix, db.Session)
	return &Store{
		mstore: blobstore.NewManagedStorage(db, rs),
	}
}

func (s *Store) challengeResponse(resp *ContentChallengeResponse) error {
	id, err := strconv.ParseInt(resp.RequestId, 10, 64)
	if err != nil {
		return errgo.Newf("invalid request id %q", id)
	}
	return s.mstore.ProofOfAccessResponse(blobstore.NewPutResponse(id, resp.Hash))
}

// Put tries to stream the content from the given reader into blob
// storage, with the provided name. The content should have the given
// size and hash. If the content is already in the store, a
// ContentChallengeError is returned containing a challenge that must be
// satisfied by a client to prove that they have access to the content.
// If the proof has already been acquired, it should be passed in as the
// proof argument.
func (s *Store) Put(r io.Reader, name string, size int64, hash string, proof *ContentChallengeResponse) (*ContentChallenge, error) {
	if proof != nil {
		err := s.challengeResponse(proof)
		if err == nil {
			return nil, nil
		}
		if err != blobstore.ErrResourceDeleted {
			return nil, errgo.Mask(err)
		}
		// The blob has been deleted since the challenge
		// was created, so continue on with uploading
		// the content as if there was no previous challenge.
	}
	resp, err := s.mstore.PutForEnvironmentRequest("", name, hash)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := s.mstore.PutForEnvironmentAndCheckHash("", name, r, size, hash); err != nil {
				return nil, errgo.Mask(err)
			}
			return nil, nil
		}
		return nil, err
	}
	return &ContentChallenge{
		RequestId:   fmt.Sprint(resp.RequestId),
		RangeStart:  resp.RangeStart,
		RangeLength: resp.RangeLength,
	}, nil
}

// PutUnchallenged stream the content from the given reader into blob
// storage, with the provided name. The content should have the given
// size and hash. In this case a challenge is never returned and a proof
// is not required.
func (s *Store) PutUnchallenged(r io.Reader, name string, size int64, hash string) error {
	return s.mstore.PutForEnvironmentAndCheckHash("", name, r, size, hash)
}

// Open opens the entry with the given name.
func (s *Store) Open(name string) (ReadSeekCloser, int64, error) {
	r, length, err := s.mstore.GetForEnvironment("", name)
	if err != nil {
		return nil, 0, errgo.Mask(err)
	}
	return r.(ReadSeekCloser), length, nil
}

// Remove the given name from the Store.
func (s *Store) Remove(name string) error {
	return s.mstore.RemoveForEnvironment("", name)
}
