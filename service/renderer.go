// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"runtime"

	"github.com/juju/utils"

	"github.com/juju/juju/cloudinit"
)

// TODO(ericsnow) Merge with cloudinit/renderers.go and move to utils.

type renderer struct {
	cloudinit.Renderer

	exeSuffix string
	shquote   func(path string) string
}

func newRenderer(os string) renderer {
	if os == "" {
		os = runtime.GOOS
	}

	switch os {
	case "windows":
		return renderer{
			Renderer:  &cloudinit.WindowsRenderer{},
			exeSuffix: ".exe",
			shquote:   func(path string) string { return `"` + path + `"` },
		}
	default:
		return renderer{
			Renderer:  &cloudinit.UbuntuRenderer{},
			exeSuffix: "",
			shquote:   utils.ShQuote,
		}
	}
}
