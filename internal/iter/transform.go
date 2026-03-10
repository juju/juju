// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package iter

import stditer "iter"

// TransformSeq applies the given tranformation func over the sequence s
// producing a new sequence of O values.
func TransformSeq[V, O any](s stditer.Seq[V], f func(V) O) stditer.Seq[O] {
	return func(yield func(O) bool) {
		s(func(v V) bool {
			return yield(f(v))
		})
	}
}
