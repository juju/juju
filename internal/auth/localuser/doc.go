// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package localuser provides an authenticator implementation against the
// controllers locally maintained users database. This package considers
// authentication of users in the context of a model.
//
// The decision to include model as part of the contract was done to potentially
// consider the fact model's may own their users and authentication means one
// day. By also including model in the scope it helps enforce stricter multi
// tennancy within Juju.
//
// This package will not attempt nor perform authentication for users that are
// considered external to the controller yet appear in the controller's user
// records.
package localuser
