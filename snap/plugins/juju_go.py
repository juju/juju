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
import json
import os
import subprocess
import sys
from typing import Any, Dict, List, Set

from snapcraft.plugins.v2 import PluginV2

# The part running this plugin. Must match the part name in
# snap/snapcraft.yaml: the plugin locates the part's build directory for the
# metadata file that set-build-metadata.sh later reads.
PART_NAME = "juju"


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

    @staticmethod
    def _project_dir() -> str:
        project_dir = os.environ.get("SNAPCRAFT_PROJECT_DIR")
        if project_dir:
            return project_dir
        return os.path.dirname(
            os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        )

    def _metadata_file(self) -> str:
        project_dir = self._project_dir()
        part_build = os.environ.get("SNAPCRAFT_PART_BUILD")
        if not part_build:
            # get_build_environment runs before Snapcraft exports the step
            # environment, so fall back to the part's build directory.
            part_build = os.path.join(
                project_dir, "parts", PART_NAME, "build"
            )
        return os.path.join(part_build, "build-metadata.json")

    def _build_metadata(self, metadata_file: str) -> Dict[str, str]:
        project_dir = self._project_dir()
        helper = os.path.join(project_dir, "scripts", "snap", "build_metadata.py")
        try:
            os.makedirs(os.path.dirname(metadata_file), exist_ok=True)
            output = subprocess.check_output(
                [
                    sys.executable,
                    helper,
                    "--project-dir",
                    project_dir,
                    "--format",
                    "json",
                    "--output",
                    metadata_file,
                ],
                text=True,
            )
        except (OSError, subprocess.CalledProcessError) as e:
            raise RuntimeError(
                f"failed to compute Juju snap build metadata: {e}"
            ) from e
        return json.loads(output)

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

        metadata_file = self._metadata_file()
        metadata = self._build_metadata(metadata_file)
        external_strings = {
            "github.com/juju/juju/version.GitCommit": metadata["git_commit"],
            "github.com/juju/juju/version.GitTreeState": metadata[
                "git_tree_state"
            ],
            "github.com/juju/juju/version.build": metadata["build"],
            "github.com/juju/juju/cloudconfig/podcfg.JujudOCINamespace": metadata[
                "oci_namespace"
            ],
        }
        for key, value in external_strings.items():
            ld_flags += f" -X {key}={value}"
        env.update({
            "JUJU_BUILD_METADATA_FILE": metadata_file,
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
