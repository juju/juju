#!/usr/bin/env python
import os
import re
import subprocess
import sys
import yaml


def get_selected_environment(selected):
    if selected is None:
        selected = default_env()
    return get_environments()[selected], selected


def call_with_nova_env(args, environment):
    new_environ = dict(os.environ)
    new_environ.update(translate_to_env(environment))
    subprocess.call(args, env=new_environ)


def get_stateserver_ips(environment, name):
    new_environ = dict(os.environ)
    new_environ.update(translate_to_env(environment))
    listing = subprocess.check_output(['nova', 'list'], env=new_environ)
    match = re.search('juju-%s-machine-0[^=]*=([^|]*)' % name, listing)
    ips = match.group(1).strip().split(', ')
    return ips


def get_environments():
    """Return the environments for juju."""
    home = os.environ.get('JUJU_HOME')
    if home is None:
        home = os.path.join(os.environ.get('HOME'), '.juju')
    with open(os.path.join(home, 'environments.yaml')) as env:
        return yaml.safe_load(env)['environments']


def default_env():
    """Determine Juju's default environment."""
    output = subprocess.check_output(['juju', 'switch'])
    return re.search('\"(.*)\"', output).group(1)


def translate_to_env(current_env):
    """Translate openstack settings to environment variables."""
    # Region doesn't follow the mapping for other vars.
    new_environ = {'OS_REGION_NAME': current_env['region']}
    for key in ['username', 'password', 'tenant-name', 'auth-url']:
        new_environ['OS_' + key.upper().replace('-', '_')] = current_env[key]
    return new_environ
