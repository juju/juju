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

func (g GenerationVersion) String() string {
	return string(g)
}

const generationKeySuffix = "#next"

// NextGenerationKey adds a suffix to the input key that designates it as being
// for "next" generation config, and returns the result.
func NextGenerationKey(key string) string {
	return key + generationKeySuffix
}

// GenerationApplication represents changes to an application
// made under a generation.
type GenerationApplication struct {
	// ApplicationsName is the name of the application.
	ApplicationName string `yaml:"application"`

	// Units are the names of units of the application that have been
	// moved to the generation.
	Units []string `yaml:"units"`

	// Config changes are the differing configuration values between this
	// generation and the current.
	// TODO (manadart 2018-02-22) This data-type will evolve as more aspects
	// of the application are made generational.
	ConfigChanges map[string]interface{} `yaml:"config"`
}

// Generation represents detail of a model generation including config changes.
type Generation struct {
	// Created is the formatted time at generation creation.
	Created string `yaml:"created"`

	// Created is the user who created the generation.
	CreatedBy string `yaml:"created-by"`

	// Applications is a collection of applications with changes in this
	// generation including advanced units and modified configuration.
	Applications []GenerationApplication `yaml:"applications"`
}

// GenerationSummaries is a type alias for a representation
// of changes-by-generation.
type GenerationSummaries = map[GenerationVersion]Generation
