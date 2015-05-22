import errno
import os
import re
import subprocess
import sys
import yaml


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
    try:
        with open(single_name) as env:
            return yaml.safe_load(env)['bootstrap-config']
    except IOError as e:
        if e.errno != errno.ENOENT:
            raise


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
    if current_env['type'] != 'openstack':
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
        new_environ['EC2_' + key.upper().replace('-', '_')] = current_env[key]
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


def describe_substrate(config):
    if config['type'] == 'local':
        return {
            'kvm': 'KVM (local)',
            'lxc': 'LXC (local)'
        }[config.get('container', 'lxc')]
    elif config['type'] == 'openstack':
        if config['auth-url'].endswith('hpcloudsvc.com:35357/v2.0/'):
            return 'HPCloud'
        elif config['auth-url'] == (
                'https://keystone.canonistack.canonical.com:443/v2.0/'):
            return 'Canonistack'
        else:
            return 'Openstack'
    try:
        return {
            'ec2': 'AWS',
            'joyent': 'Joyent',
            'azure': 'Azure',
            'maas': 'MAAS',
        }[config['type']]
    except KeyError:
        return config['type']


def setup_juju_path(juju_path):
    """Ensure the binaries and scripts under test are found first."""
    full_path = os.path.abspath(juju_path)
    if not os.path.isdir(full_path):
        raise ValueError("The juju_path does not exist: %s" % full_path)
    os.environ['PATH'] = '%s:%s' % (full_path, os.environ['PATH'])
    sys.path.insert(0, full_path)
