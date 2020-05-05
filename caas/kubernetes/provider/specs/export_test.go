// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

var (
	ParsePodSpecV2       = parsePodSpecV2
	ParsePodSpecLegacy   = parsePodSpecLegacy
	ParsePodSpecForTest  = parsePodSpec
	NewYAMLOrJSONDecoder = newYAMLOrJSONDecoder
	NewDeployer          = newDeployer
)

type (
	K8sContainers = k8sContainers
	K8sContainer  = k8sContainer
	ParserType    = parserType
)
