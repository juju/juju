// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hook

type listSource struct {
	NoUpdates
	hooks []Info
}

func (q *listSource) Empty() bool {
	return len(q.hooks) == 0
}

func (q *listSource) Next() Info {
	if q.Empty() {
		panic("Source is empty")
	}
	return q.hooks[0]
}

func (q *listSource) Pop() {
	if q.Empty() {
		panic("Source is empty")
	}
	q.hooks = q.hooks[1:]
}

// NewListSource returns a Source that generates only the supplied hooks, in
// order; and which cannot be updated.
func NewListSource(list []Info) Source {
	source := &listSource{hooks: make([]Info, len(list))}
	copy(source.hooks, list)
	return source
}
