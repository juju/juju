// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

// LabelsForApp returns the labels that should be on a k8s object for a given
// application name
func LabelsForApp(name string) map[string]string {
	return map[string]string{
		labelApplication: name,
	}
}

// LabelsForModel returns the labels that should be on a k8s object for a given
// model name
func LabelsForModel(name string) map[string]string {
	return map[string]string{
		labelModel: name,
	}
}

// AppendLabels adds the labels defined in src to dest returning the result.
// Overlapping keys in sources maps are overwritten by the very last defined
// value for a duplicate key.
func AppendLabels(dest map[string]string, sources ...map[string]string) map[string]string {
	if dest == nil {
		dest = map[string]string{}
	}
	if sources == nil {
		return dest
	}
	for _, s := range sources {
		for k, v := range s {
			dest[k] = v
		}
	}
	return dest
}
