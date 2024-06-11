// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package keys provides the domain needed for configuring authorised keys on a
// model for a user.
//
// This package still uses model config as an implementation detail for storing
// user authorised keys. Because of this implementation detail we have to
// discard the association between a user and their authorised keys.
//
// Also because of the way we are persisting this data we also don't have any
// transactionality when modifying authorised keys potentially resulting in data
// that may not be accurate.
//
// Both of the problems described above exist with the original implementation
// using Mongo and model config.
package keys
