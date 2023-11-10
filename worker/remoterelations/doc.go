// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package remoterelations contains workers which manage the operation of cross model relations.
// The workers run on both the consuming and offering models.
// The consuming side pushes relation updates from the consumer application to the model containing
// the offer. It also polls the offered application to record relation changes from the offer into
// the consuming model.
package remoterelations
