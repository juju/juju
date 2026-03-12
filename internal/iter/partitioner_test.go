// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package iter

import (
	"fmt"
	"testing"

	"github.com/juju/tc"
)

// UserRecord is a testing type to define a mock user value to process with
// [Partitioner]. UserRecord implements the [Partitionable] interface.
type UserRecord struct {
	userID int
	action string
}

// partitionerSuite encapsulates a suite of tests to assert the interface
// offered by [Partitioner].
type partitionerSuite struct{}

// partitionT is a testing int value that can be used with [Partitioner].
// partitionT implements the [Partitionable] interface.
type partitionT int

// Partition returns the int value of [partitionT]. Implements the [Partitioner]
// interface.
func (p partitionT) Partition() int {
	return int(p)
}

func (u UserRecord) Partition() int {
	return u.userID
}

// ExamplePartitioner demonstrates basic usage of the Partitioner to group
// a pre-sorted sequence of values by their partition keys.
func ExamplePartitioner() {
	// records are ordered on the access pattern of each partition.
	records := []UserRecord{
		{userID: 1, action: "login"},
		{userID: 1, action: "logout"},
		{userID: 1, action: "update"},
		{userID: 2, action: "login"},
		{userID: 2, action: "logout"},
		{userID: 3, action: "login"},
		{userID: 3, action: "logout"},
		{userID: 3, action: "update"},
		{userID: 3, action: "delete"},
	}

	// Create a partitioner
	partitioner := NewPartitioner(records)
	defer partitioner.Close()

	// Collect all records for user 1
	user1Records := partitioner.CollectNextPart(1)
	fmt.Printf("User 1: %d records\n", len(user1Records))

	// Collect all records for user 2
	user2Records := partitioner.CollectNextPart(2)
	fmt.Printf("User 2: %d records\n", len(user2Records))

	// Collect all records for user 3
	user3Records := partitioner.CollectNextPart(3)
	fmt.Printf("User 3: %d records\n", len(user3Records))

	// Output:
	// User 1: 3 records
	// User 2: 2 records
	// User 3: 4 records
}

// ExamplePartitioner_NextPart demonstrates using [Partitioner.NextPart] to
// iterate over a partition with fine-grained control.
func ExamplePartitioner_NextPart() {
	// Input sorted by partition key
	data := []partitionT{1, 1, 2, 2, 2}

	partitioner := NewPartitioner(data)
	defer partitioner.Close()

	// Process partition 1 with custom logic
	fmt.Println("Partition 1:")
	for v := range partitioner.NextPart(1) {
		fmt.Printf("  Value: %d\n", v)
	}

	// Process partition 2
	fmt.Println("Partition 2:")
	count := 0
	for v := range partitioner.NextPart(2) {
		count++
		fmt.Printf("  Value: %d\n", v)
		if count == 2 {
			// Can break early if needed. However all values mus be consumed
			// before calling NextPart again.
			break
		}
	}

	// Output:
	// Partition 1:
	//   Value: 1
	//   Value: 1
	// Partition 2:
	//   Value: 2
	//   Value: 2
}

// TestPartitionerSuite runs all of the tests contained within
// [partitionerSuite].
func TestPartitionerSuite(t *testing.T) {
	tc.Run(t, partitionerSuite{})
}

// TestDense verifies that the Partitioner correctly handles a densely packed
// sequence where most partition keys are present (0-8), with varying counts
// per partition. It asserts that each partition returns the correct number of
// elements when accessed sequentially in ascending order.
func (partitionerSuite) TestDense(c *tc.C) {
	partitions := []partitionT{1, 1, 2, 3, 4, 5, 5, 5, 6, 7, 8, 8, 8, 8}

	collected := map[int]int{}
	partitioner := NewPartitioner(partitions)
	defer partitioner.Close()

	for i := range 9 {
		collected[i] = len(partitioner.CollectNextPart(i))
	}

	c.Check(collected, tc.DeepEquals, map[int]int{
		0: 0,
		1: 2,
		2: 1,
		3: 1,
		4: 1,
		5: 3,
		6: 1,
		7: 1,
		8: 4,
	})
}

// TestEmpty verifies that the Partitioner correctly handles an empty input
// sequence. It asserts that requesting any partition key returns zero elements,
// demonstrating graceful handling of the empty case without errors.
func (partitionerSuite) TestEmpty(c *tc.C) {
	partitions := []partitionT{}

	collected := map[int]int{}
	partitioner := NewPartitioner(partitions)
	defer partitioner.Close()

	for i := range 9 {
		collected[i] = len(partitioner.CollectNextPart(i))
	}

	c.Check(collected, tc.DeepEquals, map[int]int{
		0: 0,
		1: 0,
		2: 0,
		3: 0,
		4: 0,
		5: 0,
		6: 0,
		7: 0,
		8: 0,
	})
}

// TestSparse verifies that the Partitioner correctly handles a sparse sequence
// where only some partition keys are present (1, 4, 8) while others (0, 2, 3,
// 5, 6, 7) are absent. It asserts that missing partitions return zero elements
// and present partitions return the correct counts, demonstrating that gaps in
// partition keys are handled correctly.
func (partitionerSuite) TestSparse(c *tc.C) {
	partitions := []partitionT{1, 1, 4, 8, 8, 8, 8}

	collected := map[int]int{}
	partitioner := NewPartitioner(partitions)
	defer partitioner.Close()

	for i := range 9 {
		collected[i] = len(partitioner.CollectNextPart(i))
	}

	c.Check(collected, tc.DeepEquals, map[int]int{
		0: 0,
		1: 2,
		2: 0,
		3: 0,
		4: 1,
		5: 0,
		6: 0,
		7: 0,
		8: 4,
	})
}

// TestReentrant verifies that a sequence iterator returned by NextPart can be
// called multiple times (re-entered) with a yield function that returns false.
// It asserts that each call to the sequence yields exactly one element until
// all elements are exhausted, and subsequent calls yield nothing. This tests
// the iterator's ability to resume from where it left off after early exits.
func (partitionerSuite) TestReentrant(c *tc.C) {
	partitions := []partitionT{1, 1, 1}

	partitioner := NewPartitioner(partitions)
	defer partitioner.Close()

	seq := partitioner.NextPart(1)

	var calledCount int
	yield := func(v partitionT) bool {
		calledCount++
		c.Check(v, tc.Equals, partitionT(1))
		return false
	}
	seq(yield)
	seq(yield)
	seq(yield)
	c.Check(calledCount, tc.Equals, 3)
	calledCount = 0

	// Should be no more elements to process.
	seq(yield)
	c.Check(calledCount, tc.Equals, 0)
}

// TestEarlyExitWithPeekedValue verifies that the Partitioner correctly handles
// early exit (yield returning false) when consuming a peeked value. After fully
// consuming partition 1, the Partitioner peeks ahead and stores the first value
// of partition 2. This test asserts that when the sequence for partition 2 is
// called multiple times with a yield function that returns false, it correctly
// yields the peeked value on each call until exhausted. This covers the critical
// code path where a cached/peeked value from a previous partition is yielded and
// the consumer exits early.
func (partitionerSuite) TestEarlyExitWithPeekedValue(c *tc.C) {
	partitions := []partitionT{1, 1, 2, 2}

	partitioner := NewPartitioner(partitions)
	defer partitioner.Close()

	// Fully consume partition 1 - this will peek the first value from partition 2
	partition1 := partitioner.CollectNextPart(1)
	c.Check(partition1, tc.HasLen, 2)

	var calledCount int
	yield := func(v partitionT) bool {
		calledCount++
		c.Check(v, tc.Equals, partitionT(2))
		return false
	}
	seq := partitioner.NextPart(2)
	seq(yield)
	seq(yield)
	c.Check(calledCount, tc.Equals, 2)
}
