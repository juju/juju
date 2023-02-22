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

"""The juju-dqlite plugin used for snapping juju.

This plugin uses the common plugin keywords as well as those for "sources".
For more information check the 'plugins' topic for the former and the
'sources' topic for the latter.
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
                "curl-channel": {
                    "type": "string",
                    "default": "latest/stable",
                },
            },
            "required": ["source"],
        }

    def get_build_snaps(self) -> Set[str]:
        return {f"curl/{self.options.curl_channel}"}
    
    def get_build_packages(self) -> Set[str]:
        return {"gcc"}
    
    def get_build_environment(self) -> Dict[str, str]:
        return {}
    
    def get_build_commands(self) -> List[str]:
        return [
            f'curl -o ${{SNAPCRAFT_PART_INSTALL}}/dqlite.tar.bz2 https://dqlite-static-libs.s3.amazonaws.com/latest-dqlite-deps-${{SNAP_ARCH}}.tar.bz2',
            f'tar -C ${{SNAPCRAFT_PART_INSTALL}} -xjf ${{SNAPCRAFT_PART_INSTALL}}/dqlite.tar.bz2',
        ]
