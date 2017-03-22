# Copyright 2013-2017 Canonical Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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


def get_jenv_path(juju_home, name):
    return os.path.join(juju_home, 'environments', '%s.jenv' % name)


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
