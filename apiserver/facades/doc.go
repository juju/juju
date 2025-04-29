// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package facades defines the facades. A facade is a collection of API server
// methods organised around CLI or worker interactions. Versioning is at facade
// granularity, and a facade’s most fundamental responsibility is to validate
// incoming API calls and ensure that they’re sensible before taking action.

package facades
