# -*- Mode:Python; indent-tabs-mode:nil; tab-width:4 -*-
#
# Copyright (C) 2020 Canonical Ltd
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 3 as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

"""The juju-go plugin used for snapping juju.

This plugin uses the common plugin keywords as well as those for "sources".
For more information check the 'plugins' topic for the former and the
'sources' topic for the latter.

Additionally, this plugin uses the following plugin-specific keywords:

    - go-channel
      (string, default: latest/stable)
      The Snap Store channel to install go from.

    - go-buildtags
      (list of strings)
      Tags to use during the go build. Default is not to use any build tags.
"""
from typing import Any, Dict, List, Set

from snapcraft.plugins.v2 import PluginV2


class PluginImpl(PluginV2):
    @classmethod
    def get_schema(cls) -> Dict[str, Any]:
        return {
            "$schema": "http://json-schema.org/draft-04/schema#",
            "type": "object",
            "additionalProperties": False,
            "properties": {
                "go-channel": {"type": "string", "default": "latest/stable"},
                "go-buildtags": {
                    "type": "array",
                    "uniqueItems": True,
                    "items": {"type": "string"},
                    "default": [],
                },
                "go-packages": {
                    "type": "array",
                    "minitems": 1,
                    "uniqueItems": True,
                    "items": {"type": "string"},
                    "default": ["./..."],
                },
                "go-external-strings": {
                    "type": "object",
                    "additionalProperties": {"type": "string"},
                    "default": {},
                },
                "go-static": {
                    "type": "boolean",
                    "default": False,
                },
                "go-strip": {
                    "type": "boolean",
                    "default": False,
                },
            },
            "required": ["source"],
        }

    def get_build_snaps(self) -> Set[str]:
        return {f"go/{self.options.go_channel}"}

    def get_build_packages(self) -> Set[str]:
        return {"gcc"}

    def get_build_environment(self) -> Dict[str, str]:
        env = {
            "GOBIN": "${SNAPCRAFT_PART_INSTALL}/bin",
        }
        if self.options.go_static:
            env.update({
                "CGO_ENABLED": "0"
            })

        ld_flags = ''
        if self.options.go_strip:
            ld_flags += '-s -w '
        if self.options.go_static:
            ld_flags += '-extldflags "-static" '
        ld_flags = ld_flags.strip()

        if len(self.options.go_external_strings) > 0:
            for k, v in self.options.go_external_strings.items():
                ld_flags += f' -X {k}={v}'
        env.update({
            "SNAPCRAFT_GO_LDFLAGS": f'{ld_flags}'
        })
        return env

    def get_build_commands(self) -> List[str]:
        if self.options.go_buildtags:
            tags = "-tags={}".format(",".join(self.options.go_buildtags))
        else:
            tags = ""

        cmd = f'go install -p "${{SNAPCRAFT_PARALLEL_BUILD_COUNT}}" {tags} -ldflags "${{SNAPCRAFT_GO_LDFLAGS}}"'
        for go_package in self.options.go_packages:
            cmd += f" {go_package}"

        return [
            "go mod download",
            cmd,
        ]
