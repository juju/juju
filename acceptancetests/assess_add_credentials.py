#!/usr/bin/env python3
"""Assess proper functionality of juju add-credential."""

from __future__ import print_function

import argparse
import logging
import os
import shutil
import sys
import yaml
import pexpect

from deploy_stack import (
    BootstrapManager,
)
from utility import (
    configure_logging,
    add_basic_testing_arguments,
    JujuAssertionError,
    temp_dir,
)

__metaclass__ = type

log = logging.getLogger("assess_add_credentials")

juju_home = os.environ.get('JUJU_HOME')


def assess_add_credentials(args):
    """Tests if juju's add-credentials command works as expected.

    Adds credentials from our real source to our juju client and tests if
    that client can bootstrap.

    :param client: Client object used in bootstrap check
    :param args: Test arguments
    """

    if 'vmaas' in args.env:
        env = 'maas'
    elif 'gce' in args.env:
        env = 'google'
    elif 'aws' in args.env:
        env = 'aws'
    else:
        env = args.env.split('parallel-')[1]

    # If no cloud-city path is given, we grab the credentials from env
    # juju_home.  Else we override the path where we read the credentials yaml
    # for testing purposes.
    if args.juju_home is not None:
        cred = get_credentials(env, args.juju_home)
    else:
        cred = get_credentials(env)

        # Fix the keypath to reflect local username
    key_path = cred['credentials'].get('private-key-path')
    if key_path:
        cred['credentials']['private-key-path'] = os.path.join(
            juju_home, '{}-key'.format(env))

    verify_add_credentials(args, env, cred)
    verify_credentials_match(env, cred)
    verify_bootstrap(args)

    log.info('SUCCESS')


def verify_add_credentials(args, env, cred):
    """Adds the supplied credential to juju with 'juju add-credential'.

    :param args: Testing arguments
    :param env: String environment name
    :param cred: Dict of credential information
    """
    testing_variations = {
        'aws': add_aws,
        'google': add_gce,
        'rackspace': add_rackspace,
        'maas': add_maas,
        'azure': add_azure
    }

    log.info("Adding {} credential from /cloud-city/credentials.yaml "
             "into testing instance".format(args.env))
    with pexpect.spawn('juju add-credential {} --client'.format(env)) as child:
        try:
            testing_variations[env](child, env, cred)
        except pexpect.TIMEOUT:
            log.error('Buffer: {}'.format(child.buffer))
            log.error('Before: {}'.format(child.before))
            raise Exception(
                'Registering user failed: pexpect session timed out')


def get_credentials(env, creds_path=juju_home):
    """Gets the stored test credentials.

    :return: Dict of credential information
    """
    with open(os.path.join(creds_path, 'credentials.yaml')) as f:
        creds_dict = yaml.load(f)
    cred = creds_dict['credentials'][env]
    return cred


def verify_bootstrap(args):
    """Verify the client can bootstrap with the newly added credentials

    :param args: Testing arguments
    """
    env_file = os.path.join(
        os.environ['HOME'], 'cloud-city', 'environments.yaml')
    shutil.copy(env_file, os.environ['JUJU_DATA'])
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        log.info('Bootstrap successfull, tearing down client')


def verify_credentials_match(env, cred):
    """Verify the credentials entered match the stored credentials.

    :param env: String environment name
    :param cred: Dict of credential information
    """
    with open(os.path.join(os.environ['JUJU_DATA'], 'credentials.yaml')) as f:
        test_creds = yaml.load(f)
        test_creds = test_creds['credentials'][env][env]
    if not test_creds == cred['credentials']:
        error = 'Credential miss-match after manual add'
        raise JujuAssertionError(error)


def end_session(session):
    """Convenience function to check a pexpect session has properly closed.
    """
    session.expect(pexpect.EOF)
    session.close()
    if session.exitstatus != 0:
        log.error('Buffer: {}'.format(session.buffer))
        log.error('Before: {}'.format(session.before))
        raise Exception('pexpect process exited with {}'.format(
            session.exitstatus))


def add_aws(child, env, cred):
    """Adds credentials for AWS to test client using real credentials.

    :param child: pexpect.spawn object of the juju add-credential command
    :param env: String environment name
    :param cred: Dict of credential information
    """
    access_key = cred['credentials']['access-key']
    secret_key = cred['credentials']['secret-key']

    child.expect('Enter credential name:')
    child.sendline(env)
    child.expect('Regions')
    child.sendline()
    child.expect('Enter access-key:')
    child.sendline(access_key)
    child.expect('Enter secret-key:')
    child.sendline(secret_key)
    end_session(child)
    log.info('Added AWS credential')


def add_gce(child, env, cred):
    """Adds credentials for GCE to test client using real credentials.

    :param child: pexpect.spawn object of the juju add-credential command
    :param env: String environment name
    :param cred: Dict of credential information
    """
    auth_type = cred['credentials']['auth-type']
    project_id = cred['credentials']['project-id']
    private_key = cred['credentials']['private-key']
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
    child.send(private_key)
    child.sendline('')
    child.expect('Enter project-id:')
    child.sendline(project_id)
    end_session(child)
    log.info('Added GCE credential')


def add_rackspace(child, env, cred):
    """Adds credentials for Rackspace to test client using real credentials.

    :param child: pexpect.spawn object of the juju add-credential command
    :param env: String environment name
    :param cred: Dict of credential information
    """
    username = cred['credentials']['username']
    password = cred['credentials']['password']
    tenant_name = cred['credentials']['tenant-name']

    child.expect('Enter credential name:')
    child.sendline(env)
    child.expect('Enter username:')
    child.sendline(username)
    child.expect('Enter password:')
    child.sendline(password)
    child.expect('Enter tenant-name:')
    child.sendline(tenant_name)
    end_session(child)
    log.info('Added Rackspace credential')


def add_maas(child, env, cred):
    """Adds credentials for MaaS to test client using real credentials.

    :param child: pexpect.spawn object of the juju add-credential command
    :param env: String environment name
    :param cred: Dict of credential information
    """
    maas_oauth = cred['credentials']['maas-oauth']

    child.expect('Enter credential name:')
    child.sendline(env)
    child.expect('Enter maas-oauth:')
    child.sendline(maas_oauth)
    end_session(child)
    log.info('Added MaaS credential')


def add_azure(child, env, cred):
    """Adds credentials for Azure to test client using real credentials.

    :param child: pexpect.spawn object of the juju add-credential command
    :param env: String environment name
    :param cred: Dict of credential information
    """
    auth_type = cred['credentials']['auth-type']
    application_id = cred['credentials']['application-id']
    subscription_id = cred['credentials']['subscription-id']
    application_password = cred['credentials']['application-password']

    child.expect('Enter credential name:')
    child.sendline(env)
    child.expect('Select auth-type:')
    child.sendline(auth_type)
    child.expect('Enter application-id:')
    child.sendline(application_id)
    child.expect('Enter subscription-id:')
    child.sendline(subscription_id)
    child.expect('Enter application-password:')
    child.sendline(application_password)
    end_session(child)
    log.info('Added Azure credential')


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Test if juju properly adds credentials with the '
                    'add-credential command.')
    add_basic_testing_arguments(parser, existing=False)
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
