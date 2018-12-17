// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

// GenerationVersion indicates a generation to use for model config.
type GenerationVersion string

const (
	// GenerationCurrent indicates the current generation for model config.
	// This is the default state of a model.
	GenerationCurrent GenerationVersion = "current"

	// GenerationNext indicates the next generation of model config.
	// Models with an active "next" generation apply the generation config
	// selectively to units added to the generation.
	GenerationNext GenerationVersion = "next"
)
