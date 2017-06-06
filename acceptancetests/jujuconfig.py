import os
import re
import subprocess


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
