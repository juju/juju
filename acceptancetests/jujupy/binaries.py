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
import os

from deploy_stack import (
    BootstrapManager
)
from jujupy.client import (
    client_from_config
)

__metaclass__ = type

log = logging.getLogger(__name__)


def get_stable_juju(args, stable_juju_bin=None):
    """Get the installed stable version of juju.

    We need a stable version of juju to boostrap and migrate from to the newer
    development version of juju.

    If no juju path is provided try some well known paths in an attempt to find
    a system installed juju that will suffice.
    Note. this function does not check if the found juju is a suitable version
    for this test, just that the binary exists and is executable.

    :param stable_juju_bin: Path to the juju binary to be used and considered
      stable
    :raises RuntimeError: If there is no valid installation of juju available.
    :return: BootstrapManager object for the stable juju.
    """
    if stable_juju_bin is not None:
        try:
            client = client_from_config(
                args.env,
                stable_juju_bin,
                debug=args.debug)
            log.info('Using {} for stable juju'.format(stable_juju_bin))
            return BootstrapManager.from_client(args, client)
        except OSError as e:
            raise RuntimeError(
                'Provided stable juju path is not valid: {}'.format(e))
    known_juju_paths = (
        '{}/bin/juju'.format(os.environ.get('GOPATH')),
        '/snap/bin/juju',
        '/usr/bin/juju')

    for path in known_juju_paths:
        try:
            client = client_from_config(
                args.env,
                path,
                debug=args.debug)
            log.info('Using {} for stable juju'.format(path))
            return BootstrapManager.from_client(args, client)
        except OSError:
            log.debug('Attempt at using {} failed.'.format(path))
            pass

    raise RuntimeError('Unable to get a stable system juju binary.')
