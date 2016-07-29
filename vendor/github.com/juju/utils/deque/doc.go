// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// The deque package implements an efficient double-ended queue data
// structure called Deque.
//
// Usage:
//
//    d := deque.New()
//    d.PushFront("foo")
//    d.PushBack("bar")
//    d.PushBack("123")
//    l := d.Len()          // l == 3
//    v, ok := d.PopFront() // v.(string) == "foo", ok == true
//    v, ok = d.PopFront()  // v.(string) == "bar", ok == true
//    v, ok = d.PopBack()   // v.(string) == "123", ok == true
//    v, ok = d.PopBack()   // v == nil, ok == false
//    v, ok = d.PopFront()  // v == nil, ok == false
//    l = d.Len()           // l == 0
//
// A discussion of the internals can be found at the top of deque.go.
//
package deque
