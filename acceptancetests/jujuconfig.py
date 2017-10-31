import os
import re
import subprocess


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
