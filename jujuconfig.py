import os
import re
import subprocess
import yaml

import utility


class NoSuchEnvironment(Exception):
    """Raised when a specified environment does not exist."""


def get_selected_environment(selected, allow_jenv=True):
    if selected is None:
        selected = default_env()
    if allow_jenv:
        jenv_config = get_jenv_config(get_juju_home(), selected)
        if jenv_config is not None:
            return jenv_config, selected
    environments = get_environments()
    env = environments.get(selected)
    if env is None:
        raise NoSuchEnvironment(
            'Environment "{}" does not exist.'.format(selected))
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


def get_juju_home():
    home = os.environ.get('JUJU_HOME')
    if home is None:
        home = os.path.join(os.environ.get('HOME'), '.juju')
    return home


def get_environments_path(juju_home):
    return os.path.join(juju_home, 'environments.yaml')


def get_jenv_path(juju_home, name):
    return os.path.join(juju_home, 'environments', '%s.jenv' % name)


def get_jenv_config(home, environment):
    single_name = get_jenv_path(home, environment)
    with utility.skip_on_missing_file():
        with open(single_name) as env:
            return yaml.safe_load(env)['bootstrap-config']


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


def translate_to_env(current_env):
    """Translate openstack settings to environment variables."""
    if current_env['type'] not in ('openstack', 'rackspace'):
        raise Exception('Not an openstack environment. (type: %s)' %
                        current_env['type'])
    # Region doesn't follow the mapping for other vars.
    new_environ = {'OS_REGION_NAME': current_env['region']}
    for key in ['username', 'password', 'tenant-name', 'auth-url']:
        new_environ['OS_' + key.upper().replace('-', '_')] = current_env[key]
    return new_environ


def get_euca_env(current_env):
    """Translate openstack settings to environment variables."""
    # Region doesn't follow the mapping for other vars.
    new_environ = {
        'EC2_URL': 'https://%s.ec2.amazonaws.com' % current_env['region']}
    for key in ['access-key', 'secret-key']:
        env_key = key.upper().replace('-', '_')
        new_environ['EC2_' + env_key] = current_env[key]
        new_environ['AWS_' + env_key] = current_env[key]
    return new_environ


def get_awscli_env(current_env):
    """Translate openstack settings to environment variables."""
    # Region doesn't follow the mapping for other vars.
    new_environ = {
        'AWS_ACCESS_KEY_ID': 'access-key',
        'AWS_SECRET_ACCESS_KEY': 'secret-key',
        'AWS_DEFAULT_REGION': 'region'
    }
    for key, value in new_environ.items():
        if value not in current_env:
            del new_environ[key]
        new_environ[key] = current_env[value]
    return new_environ
