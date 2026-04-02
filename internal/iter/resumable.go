// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package iter

import stditer "iter"

// ResumableSeq transforms a sequence into one that maintains state across
// multiple invocations. The returned sequence can be called repeatedly, and
// each call continues from where the previous call stopped (either by
// exhausting values or by the yield function returning false).
//
// The returned close function MUST be called when done with the sequence to
// release any resources held by the underlying iterator. Failing to call this
// may result in resource leaks.
//
// This function is a low-level building block for constructing higher-level
// iteration patterns such as partitioning, where you need to consume a sequence
// incrementally across multiple operations. It is not intended for direct use
// in typical Juju workflows.
//
// Example use case: A Partitioner uses ResumableSeq to pause iteration when
// encountering a value from a different partition, then resume on the next
// partition request.
func ResumableSeq[V any](s stditer.Seq[V]) (stditer.Seq[V], func()) {
	pullNext, pullStop := stditer.Pull(s)
	return func(yield func(V) bool) {
		for {
			v, valid := pullNext()
			if !valid {
				return
			}
			if !yield(v) {
				return
			}
		}
	}, pullStop
}
