// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//+build go1.3

package mempool

import "sync"

// A Pool is a set of temporary objects that may be individually saved and
// retrieved.
//
// It is a wrapper around sync.Pool.
type Pool sync.Pool

func (p *Pool) Put(x interface{}) {
	(*sync.Pool)(p).Put(x)
}

func (p *Pool) Get() interface{} {
	return (*sync.Pool)(p).Get()
}
