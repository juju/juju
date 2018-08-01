// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mempool implements a version of sync.Pool
// as supported in Go versions 1.3 and later.
//
// For Go versions prior to that, it uses an implementation
// that never deletes any values from the pool.
//
// If you don't need your code to compile on Go versions
// prior to 1.3, don't use this package - use sync.Pool
// instead.
package mempool
