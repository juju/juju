# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2017 Canonical Ltd.
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


"""Low-level access to Juju configuration."""

import os
import re
import subprocess

import yaml


class NoSuchEnvironment(Exception):
    """Raised when a specified environment does not exist."""


def get_selected_environment(selected):
    if selected is None:
        selected = default_env()
    environments = get_environments()
    env = environments.get(selected)
    if env is None:
        raise NoSuchEnvironment(
            'Environment "{}" does not exist.'.format(selected))
    return get_environments()[selected], selected


def get_juju_home():
    home = os.environ.get('JUJU_HOME')
    if home is None:
        home = os.path.join(os.environ.get('HOME'), '.juju')
    return home


def get_juju_data():
    """Return the configured juju data directory.

    Assumes non-Windows.  Follows Juju's algorithm.

    If JUJU_DATA is set, it is returned.
    If XDG_DATA_HOME is set, 'juju' is appended to it.
    Otherwise, .local/share/juju is appended to HOME.
    """
    juju_data = os.environ.get('JUJU_DATA')
    if juju_data is not None:
        return juju_data
    data_home = os.environ.get('XDG_DATA_HOME')
    if data_home is None:
        data_home = os.path.join(os.environ['HOME'], '.local', 'share')
    return os.path.join(data_home, 'juju')


def get_environments_path(juju_home):
    return os.path.join(juju_home, 'environments.yaml')


def get_bootstrap_config_path(juju_data_dir):
    return os.path.join(juju_data_dir, 'bootstrap-config.yaml')


def get_environments():
    """Return the environments for juju."""
    home = get_juju_home()
    with open(get_environments_path(home)) as env:
        return yaml.safe_load(env)['environments']


def default_env():
    """Determine Juju's default environment."""
    output = subprocess.check_output(['juju', 'switch'])
    match = re.search('\"(.*)\"', output)
    if match is None:
        return output.rstrip('\n')
    return match.group(1)
