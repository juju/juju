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


def get_environments_path(juju_home):
    return os.path.join(juju_home, 'environments.yaml')


def get_jenv_path(juju_home, name):
    return os.path.join(juju_home, 'environments', '%s.jenv' % name)


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
