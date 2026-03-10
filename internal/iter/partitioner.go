package iter

import (
	"cmp"
	"iter"
	stditer "iter"
	"slices"
)

// Partionable represents a value that can be grouped by a partition key.
// The Partition method returns the key that identifies which partition this
// value belongs to. Values with the same partition key are considered part
// of the same logical group.
type Partionable[T cmp.Ordered] interface {
	// Partition returns the value T that this object belons to.
	Partition() T
}

// Partitioner streams through a sequence of values, grouping them by partition
// keys. It allows you to extract subsequences (partitions) where all values
// share the same partition key, processing one partition at a time.
//
// The input sequence must be pre-sorted by partition key in access order,
// matching the order in which you'll request partitions. This requirement
// enables efficient streaming without buffering the entire sequence.
//
// Example use case: Processing database results grouped by user ID, where
// results are ordered by user ID, and you want to process all records for
// each user sequentially.
//
// Always call [Partitioner.Close] when done to release resources from the
// underlying sequence.
//
// Partitioner is not considered concurrency safe.
type Partitioner[V Partionable[T], T cmp.Ordered] struct {
	peeked   *V
	seq      stditer.Seq[V]
	seqClose func()
}

// NewPartitioner constructs a new Partitioner from the supplied slice S of
// values V. The slice S must be sorted by partition key in access order,
// matching the order in which partitions will be requested via
// [Partitioner.NextPart].
func NewPartitioner[S []V, V Partionable[T], T cmp.Ordered](s S) *Partitioner[V, T] {
	return NewPartitionerFromSeq(slices.Values(s))
}

// NewPartitionerFromSeq constructs a new Partitioner from an iterator sequence.
// The sequence must yield values sorted by partition key in access order,
// matching the order in which partitions will be requested via
// [Partitioner.NextPart].
func NewPartitionerFromSeq[V Partionable[T], T cmp.Ordered](s stditer.Seq[V]) *Partitioner[V, T] {
	seq, close := ResumableSeq(s)
	return &Partitioner[V, T]{
		seq:      seq,
		seqClose: close,
	}
}

// CollectNextPart returns all values for partition t as a slice. It is
// equivalent to calling [slices.Collect] on [Partitioner.NextPart(t)]. If no
// values exist for parition T then a slice of length zero is returned.
func (p *Partitioner[V, T]) CollectNextPart(t T) []V {
	return slices.Collect(p.NextPart(t))
}

// Close releases resources associated with the underlying [Paritioner]. It
// should be called when the Partitioner is no longer needed to prevent resource
// leaks.
//
// Paritioner should not be considered usable after a call to Close.
func (p *Partitioner[V, T]) Close() {
	p.seqClose()
}

// NextPart returns an iterator over all values in the sequence that belong to
// partition t. It consumes values from the underlying sequence until it
// encounters a value with a different partition key.
//
// Partitions must be requested in the same order as they appear in the source
// sequence. Requesting an out-of-order partition will yield no results since
// the Partitioner can only move forward through the sequence.
//
// The returned iterator is valid only until the next call to NextPart or Close.
//
// Sequences returned by NextPart are rentrant in that yielding from the
// supplied [iter.Seq] and running again will resume processing from the last
// yielded value.
func (p *Partitioner[V, T]) NextPart(t T) iter.Seq[V] {
	return func(yield func(V) bool) {
		for {
			if p.peeked == nil {
				p.seq(func(v V) bool {
					if v.Partition() != t {
						p.peeked = &v
						return false
					}
					return yield(v)
				})
				return
			}

			if (*p.peeked).Partition() != t {
				return
			}

			c := yield(*p.peeked)
			p.peeked = nil
			if !c {
				return
			}
		}
	}
}
