# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2018 Canonical Ltd.
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

# Functionality for handling installed or other juju binaries
# (including paths etc.)

from __future__ import print_function

import logging

from contextlib import contextmanager
from subprocess import CalledProcessError

__metaclass__ = type

log = logging.getLogger(__name__)


@contextmanager
def temporary_model(client, model_name):
    """Create a new model that is cleaned up once it's done with."""
    try:
        new_client = client.add_model(model_name)
        yield new_client
    finally:
        try:
            log.info('Destroying temp model "{}"'.format(model_name))
            new_client.destroy_model()
        except CalledProcessError:
            log.error('Failed to cleanup model.')
