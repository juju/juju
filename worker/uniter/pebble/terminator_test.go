// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pebble_test

import (
	"fmt"
	"sort"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/pebble/client"

	"github.com/juju/juju/worker/uniter/pebble"
)

type terminatorSuite struct{}

type mockTerminatorClient struct{
	shutdown func(*client.ShutdownOptions) error
}

var _ = gc.Suite(&terminatorSuite{})

func (m *mockTerminatorClient) Shutdown(o *client.ShutdownOptions) error {
	if m.shutdown != nil {
		return m.shutdown(o)
	}
	return nil
}

func (_ *terminatorSuite) TestShutdownContainersWithNoError(c *gc.C) {
	terminator := pebble.NewTerminator(func(container string) (pebble.TerminatorClient, error) {
		return &mockTerminatorClient{}, nil
	})

	notFinished, err := terminator.ShutdownContainers([]string{"test1", "test2", "test3"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(notFinished), gc.Equals, 0)
}

func (_ *terminatorSuite) TestShutdownContainersWithError(c *gc.C) {
	containers := []string{
		"test1",
		"test2",
		"test3",
		"test4",
		"test5",
	}

	sort.Strings(containers)
	callCount := 0
	terminator := pebble.NewTerminator(func(container string) (pebble.TerminatorClient, error) {
		return &mockTerminatorClient{
			shutdown: func(_ *client.ShutdownOptions) error {
				callCount++
				if callCount == 3 {
					return fmt.Errorf("forced error")
				}

				index := sort.SearchStrings(containers, container)
				if index == len(containers) || containers[index] != container {
					c.Fatalf("received unknown container termination for %q", container)
				}

				containers = append(containers[:index], containers[index+1:]...)

				return nil
			},
		}, nil
	})

	containersToTerm := make([]string, len(containers))
	copy(containersToTerm, containers)

	notFinished, err := terminator.ShutdownContainers(containersToTerm)
	c.Assert(err, gc.NotNil)

	sort.Strings(notFinished)
	c.Assert(notFinished, jc.DeepEquals, containers)
}
