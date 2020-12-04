# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2019 Canonical Ltd.
#
# This program is free software: you can redistribute it and/or modify it
# under the terms of the Lesser GNU General Public License version 3, as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful, but WITHOUT
# ANY WARRANTY; without even the implied warranties of MERCHANTABILITY,
# SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR PURPOSE.  See the Lesser
# GNU General Public License for more details.
#
# You should have received a copy of the Lesser GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

import json
import yaml

__metaclass__ = type


class Controllers:

    def __init__(self, info, text):
        self.info = info
        self.text = text

    @classmethod
    def from_text(cls, text):
        try:
            # Parsing as JSON is much faster than parsing as YAML, so try
            # parsing as JSON first and fall back to YAML.
            info = json.loads(text)
        except ValueError:
            info = yaml.safe_load(text)
        return cls(info, text)

    def get_controller(self, name):
        """Controller returns the controller associated with the name provided

        :param name: name associated with the controller
        """
        return Controller(self.info[name])


class Controller:

    def __init__(self, info):
        self.info = info

    def get_details(self):
        return ControllerDetails(self.info["details"])


class ControllerDetails:

    def __init__(self, info):
        self.info = info

    @property
    def agent_version(self):
        return self.info["agent-version"]

    @property
    def mongo_version(self):
        return self.info["mongo-version"]


class ControllerConfig:

    def __init__(self, cfg):
        self.cfg = cfg

    @classmethod
    def from_text(cls, text):
        try:
            # Parsing as JSON is much faster than parsing as YAML, so try
            # parsing as JSON first and fall back to YAML.
            cfg = json.loads(text)
        except ValueError:
            cfg = yaml.safe_load(text)
        return cls(cfg)

    @property
    def mongo_memory_profile(self):
        if 'mongo-memory-profile' in self.cfg:
            return self.cfg["mongo-memory-profile"]
        return "low"

    @property
    def db_snap_channel(self):
        if 'juju-db-snap-channel' in self.cfg:
            return self.cfg["juju-db-snap-channel"]
        return "4.0/stable"
