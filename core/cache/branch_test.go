// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/testing"
)

type BranchSuite struct {
	cache.EntitySuite
}

var _ = gc.Suite(&BranchSuite{})

func (s *BranchSuite) TestBranchSetDetailsPublishesCopy(c *gc.C) {
	rcv := make(chan interface{}, 1)
	unsub := s.Hub.Subscribe("branch-change", func(_ string, msg interface{}) { rcv <- msg })
	defer unsub()

	_ = s.NewBranch(branchChange)

	select {
	case msg := <-rcv:
		b, ok := msg.(cache.Branch)
		if !ok {
			c.Fatal("wrong type published; expected Branch.")
		}
		c.Check(b.Name(), gc.Equals, branchChange.Name)

	case <-time.After(testing.LongWait):
		c.Fatal("branch change message not Received")
	}
}

var branchChange = cache.BranchChange{
	ModelUUID:     "model-uuid",
	Id:            "0",
	Name:          "testing-branch",
	AssignedUnits: map[string][]string{"redis": {"redis/0", "redis/1"}},
	Config:        map[string]settings.ItemChanges{"redis": {settings.MakeAddition("password", "pass666")}},
	Created:       0,
	CreatedBy:     "test-user",
	Completed:     0,
	CompletedBy:   "different-user",
	GenerationId:  0,
}
