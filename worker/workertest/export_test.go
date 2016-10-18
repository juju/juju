// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workertest

// KillTimeout exists because it's the least unpleasant of the many options that
// let us test the package tolerably. Consider:
//
//  * we ought to actually write some tests for the claimed behaviour
//  * waiting 10s for a test to pass is stupid, we can't test with the default
//  * using a clock abstraction misses the point: it's all about wall clocks
//  * nobody's going to bother writing explicit checker config in their tests
//
// ...and so we convince ourselves that this mutable global state is a tolerable
// price to pay given the limited locus of influence.
var KillTimeout = &killTimeout
