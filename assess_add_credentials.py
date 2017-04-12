#!/usr/bin/env python
"""Assess proper functionality of juju add-credential."""

from __future__ import print_function

import argparse
import logging
import sys
import os
import yaml
import pexpect
import subprocess
import shutil

from deploy_stack import (
    BootstrapManager,
    )
from jujupy import (
    ModelClient
)
from utility import (
    configure_logging,
    add_basic_testing_arguments,
    JujuAssertionError,
    temp_dir,
    )

__metaclass__ = type


log = logging.getLogger("assess_add_credentials")


def assess_add_credentials(args):
    """Tests if juju's add-credentials command works as expected.

    Adds credentials from our real source to our juju client and tests if
    that client can bootstrap.

    :param client: Client object used in bootstrap check
    :param args: Test arguments
    """

    testing_variations = {
        'aws': add_aws,
        'google': add_gce,
        'rackspace': add_rackspace,
        'maas': add_maas,
        'joyent': add_joyent,
        'azure': add_azure
        }

    if 'vmaas' in args.env:
        env = 'maas'
    elif 'gce' in args.env:
        env = 'google'
    else:
        env = args.env.split('parallel-')[1]
    with open(os.path.join(os.environ['HOME'], 'cloud-city',
                           'credentials.yaml')) as f:
        creds_dict = yaml.load(f)
    cred = creds_dict['credentials'][env]

    key_raw = cred['credentials'].get('private-key')
    if key_raw:
        key = ''
        for line in key_raw.split('\n'):
            key += line
        cred['credentials']['private-key'] = key

    log.info("Adding {} credential from ~/cloud-city/credentials.yaml "
             "into testing instance".format(args.env))
    with pexpect.spawn('juju add-credential {}'.format(env)) as child:
        try:
            testing_variations[env](child, env, cred)
        except pexpect.TIMEOUT:
            log.error('Buffer: {}'.format(child.buffer))
            log.error('Before: {}'.format(child.before))
            raise Exception(
                'Registering user failed: pexpect session timed out')

    verify_credentials(env, cred)
    verify_bootstrap(args)

    log.info('SUCCESS')


def verify_bootstrap(args):
    env_file = os.path.join(
        os.environ['HOME'], 'cloud-city', 'environments.yaml')
    shutil.copy(env_file, os.environ['JUJU_HOME'])
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        log.info('Bootstrap successfull, tearing down client')


def verify_credentials(env, cred):
    cred_path = os.path.join(os.environ['JUJU_HOME'], 'credentials.yaml')
    with open(cred_path) as f:
        test_creds = yaml.load(f)
        test_creds = test_creds['credentials'][env][env]
    import pdb; pdb.set_trace()
    if not test_creds == cred['credentials']:
        error = 'Credential miss-match after manual add'
        raise JujuAssertionError(error)


def end_session(session):
    session.expect(pexpect.EOF)
    session.close()
    if session.exitstatus != 0:
        log.error('Buffer: {}'.format(session.buffer))
        log.error('Before: {}'.format(session.before))
        raise Exception('pexpect process exited with {}'.format(
                session.exitstatus))


def add_aws(child, env, cred):
    """Adds credentials for AWS to test client using real credentials.

    :param env: String environment name
    :param cred: Dict of credential information
    """
    auth_type = cred['credentials']['auth-type']
    access_key = cred['credentials']['access-key']
    secret_key = cred['credentials']['secret-key']

    child.expect('Enter credential name:')
    child.sendline(env)
    child.expect('Enter access-key:')
    child.sendline(access_key)
    child.expect('Enter secret-key:')
    child.sendline(secret_key)
    end_session(child)
    log.info('Added AWS credential')


def add_gce(child, env, cred):
    """Adds credentials for AWS to test client using real credentials.

    :param env: String environment name
    :param cred: Dict of credential information
    """
    auth_type = cred['credentials']['auth-type']
    project_id = cred['credentials']['project-id']
    private_key= cred['credentials']['private-key']
    client_email = cred['credentials']['client-email']
    client_id = cred['credentials']['client-id']

    child.expect('Enter credential name:')
    child.sendline(env)
    child.expect('Select auth-type:')
    child.sendline(auth_type)
    child.expect('Enter client-id:')
    child.sendline(client_id)
    child.expect('Enter client-email:')
    child.sendline(client_email)
    child.expect('Enter private-key:')
    child.sendline(private_key)
    child.expect('Enter project-id:')
    child.sendline(project_id)
    end_session(child)
    log.info('Added GCE credential')

    """
    auth-type
    project-id
    private-key
    client-email
    client-id
    """
    pass


def add_rackspace(client, cred):
    """
    auth-type
    username
    password
    tenant-name
    """
    pass


def add_maas(client, cred):
    """
    auth-type
    maas-oauth
    """
    pass


def add_joyent(client, cred):
    """
    auth-type
    algorithm
    sdc-user
    sdc-key-id
    private-key
    """
    pass


def add_azure(client, cred):
    """
    auth-type
    application-id
    private-key
    client-email
    client-id
    """
    pass


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Test if juju properly adds credentials with the '
        'add-credential command.')
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    with temp_dir() as temp:
        os.environ['JUJU_HOME'] = temp
        os.environ['JUJU_DATA'] = temp
        assess_add_credentials(args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
