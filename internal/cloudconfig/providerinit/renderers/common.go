// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package renderers

import (
	"encoding/base64"

	"github.com/juju/juju/internal/cloudconfig/cloudinit"
)

// ToBase64 just transforms whatever userdata it gets to base64 format
func ToBase64(data []byte) []byte {
	buf := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
	base64.StdEncoding.Encode(buf, data)
	return buf
}

// Decorator is a function that can be used as part of a rendering pipeline.
type Decorator func([]byte) []byte

// RenderYAML renders the given cloud-config as YAML, and then passes the
// YAML through the given decorators.
func RenderYAML(cfg cloudinit.RenderConfig, ds ...Decorator) ([]byte, error) {
	out, err := cfg.RenderYAML()
	if err != nil {
		return nil, err
	}
	return applyDecorators(out, ds), nil
}

// RenderScript renders the given cloud-config as a script, and then passes the
// script through the given decorators.
func RenderScript(cfg cloudinit.RenderConfig, ds ...Decorator) ([]byte, error) {
	out, err := cfg.RenderScript()
	if err != nil {
		return nil, err
	}
	return applyDecorators([]byte(out), ds), nil
}

func applyDecorators(out []byte, ds []Decorator) []byte {
	for _, d := range ds {
		out = d(out)
	}
	return out
}
