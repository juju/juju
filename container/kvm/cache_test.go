// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package kvm_test

import (
	"net/http"

	gc "gopkg.in/check.v1"

	. "github.com/juju/juju/container/kvm"
	"github.com/juju/juju/environs/imagedownloads"
	jc "github.com/juju/testing/checkers"
	"github.com/pkg/errors"
)

// cacheSuite is gocheck boilerplate.
type cacheSuite struct{}

// _ is gocheck boilerplate.
var _ = gc.Suite(cacheSuite{})

func (cacheSuite) TestSyncOnerErrors(c *gc.C) {
	o := fakeParams{FakeData: nil, Err: errors.New("oner failed")}
	u := fakeUpdater{}
	got := Sync(o, u)
	c.Assert(got, gc.ErrorMatches, "oner failed")
}

func (cacheSuite) TestSyncUpdaterErrors(c *gc.C) {
	o := fakeParams{FakeData: &imagedownloads.Metadata{}, Err: nil}
	u := fakeUpdater{Err: errors.New("updater failed")}
	got := Sync(o, u)
	c.Assert(got, gc.ErrorMatches, "updater failed")
}

func (cacheSuite) TestSyncSucceeds(c *gc.C) {
	o := fakeParams{FakeData: &imagedownloads.Metadata{}}
	u := fakeUpdater{}
	got := Sync(o, u)
	c.Assert(got, jc.ErrorIsNil)
}

type fakeParams struct {
	FakeData *imagedownloads.Metadata
	Err      error
}

func (f fakeParams) One() (*imagedownloads.Metadata, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.FakeData, nil
}

type fakeUpdater struct {
	md        *imagedownloads.Metadata
	fileCache *Image
	req       *http.Request
	client    *http.Client
	// Used to return an error
	Err error
}

func (f fakeUpdater) Update() error {
	if f.Err != nil {
		return f.Err
	}
	return nil
}
