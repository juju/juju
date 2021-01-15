#!/usr/bin/env python3
"""
This tests the wallet commands utilized for commercial charm billing.
These commands are linked to a ubuntu sso account, and as such, require the
user account to be setup before test execution (including authentication).
You can use charm login to do this, or let juju authenticate with a browser.
"""

from __future__ import print_function

import argparse
import json
import logging
import os
from random import randint
import shutil
import subprocess
import sys
import pexpect
from fixtures import EnvironmentVariable


from deploy_stack import (
    BootstrapManager,
    )

from utility import (
    add_basic_testing_arguments,
    temp_dir,
    JujuAssertionError,
    configure_logging,
)

__metaclass__ = type


log = logging.getLogger("assess_wallet")


def _get_new_wallet_limit(client):
    """Return available limit for new wallet"""
    wallets = json.loads(list_wallets(client))
    limit = int(wallets['total']['limit'])
    credit = int(wallets['credit'])
    log.debug('Found credit limit {}, currently used {}'.format(
        credit, limit))
    return credit - limit


def _get_wallets(client):
    return json.loads(list_wallets(client))['wallets']


def _set_wallet_value_expectations(expected_wallets, name, value):
    # Update our expectations accordingly
    for wallet in expected_wallets:
        if wallet['wallet'] == name:
            # For now, we assume we aren't spending down the wallet
            wallet['limit'] = value
            wallet['unallocated'] = value
            # .00 is appended to availible for some reason
            wallet['available'] = '{:.2f}'.format(float(value))
            log.info('Expected wallet updated: "{}" to {}'.format(name, value))


def _try_setting_wallet(client, name, value):
    try:
        output = set_wallet(client, name, value)
    except subprocess.CalledProcessError as e:
        output = [e.output.decode('utf-8'), getattr(e, 'stderr', '')]
        raise JujuAssertionError('Could not set wallet {}'.format(output))

    if b'wallet limit updated' not in output:
        raise JujuAssertionError('Error calling set-wallet {}'.format(output))


def _try_creating_wallet(client, name, value):
    try:
        create_wallet(client, name, value)
        log.info('Created new wallet "{}" with value {}'.format(name,
                                                                value))
    except subprocess.CalledProcessError as e:
        output = [e.output.decode('utf-8'), getattr(e, 'stderr', '')]
        if any('already exists' in message for message in output):
            log.info('Reusing wallet "{}" with value {}'.format(name, value))
            pass  # this will be a failure once lp:1663258 is fixed
        else:
            raise JujuAssertionError(
                'Error testing create-wallet: {}'.format(output))
    except Exception:
        raise JujuAssertionError('Added duplicate wallet')


def _try_greater_than_limit_wallet(client, name, limit):
    error_strings = {
        'pass': 'exceed the credit limit',
        'unknown': 'Error testing wallet greater than credit limit',
        'fail': 'Credit limit exceeded'
    }
    over_limit_value = str(limit + randint(1, 100))
    assert_set_wallet(client, name, over_limit_value, error_strings)


def _try_negative_wallet(client, name):
    error_strings = {
        'pass': 'Could not set wallet',
        'unknown': 'Error testing negative wallet',
        'fail': 'Negative wallet allowed'
    }
    negative_wallet_value = str(randint(-1000, -1))
    assert_set_wallet(client, name, negative_wallet_value, error_strings)


def assert_sorted_equal(found, expected):
    found = sorted(found)
    expected = sorted(expected)
    if found != expected:
        raise JujuAssertionError(
            'Found: {}\nExpected: {}'.format(found, expected))


def assert_set_wallet(client, name, limit, error_strings):
    try:
        _try_setting_wallet(client, name, limit)
    except JujuAssertionError as e:
        if error_strings['pass'] not in str(e):
            raise JujuAssertionError(
                '{}: {}'.format(error_strings['unknown'], e))
    else:
        raise JujuAssertionError(error_strings['fail'])


def create_wallet(client, name, value):
    """Create a wallet"""
    return client.get_juju_output('create-wallet', name, value,
                                  include_e=False)


def list_wallets(client):
    """Return defined wallets as json."""
    return client.get_juju_output('list-wallets', '--format', 'json',
                                  include_e=False)


def set_wallet(client, name, value):
    """Change an existing wallet's allocation."""
    return client.get_juju_output('set-wallet', name, value, include_e=False)


def show_wallet(client, name):
    """Return specified wallet as json."""
    return client.get_juju_output('show-wallet', name, '--format', 'json',
                                  include_e=False)


def assess_wallet(client):
    # Since we can't remove wallets until lp:1663258
    # is fixed, we avoid creating new random wallets and hardcode.
    # We also, zero out the previous wallet
    wallet_name = 'personal'
    _try_setting_wallet(client, wallet_name, '0')

    wallet_limit = _get_new_wallet_limit(client)
    assess_wallet_limit(wallet_limit)

    expected_wallets = _get_wallets(client)
    wallet_value = str(randint(1, wallet_limit / 2))
    assess_create_wallet(client, wallet_name, wallet_value, wallet_limit)

    wallet_value = str(randint(wallet_limit / 2 + 1, wallet_limit))
    assess_set_wallet(client, wallet_name, wallet_value, wallet_limit)
    assess_show_wallet(client, wallet_name, wallet_value)

    _set_wallet_value_expectations(expected_wallets, wallet_name, wallet_value)
    assess_list_wallets(client, expected_wallets)


def assess_wallet_limit(wallet_limit):
    log.info('Assessing wallet limit {}'.format(wallet_limit))

    if wallet_limit < 0:
        raise JujuAssertionError(
            'Negative Wallet Limit {}'.format(wallet_limit))


def assess_create_wallet(client, wallet_name, wallet_value, wallet_limit):
    """Test create-wallet command"""
    log.info('create-wallet "{}" with value {}, limit {}'.format(wallet_name,
                                                                 wallet_value,
                                                                 wallet_limit))

    # Do this twice, to ensure wallet exists and we can check for
    # duplicate message. Ideally, once lp:1663258 is fixed, we will
    # assert on initial wallet creation as well.
    _try_creating_wallet(client, wallet_name, wallet_value)

    log.info('Trying duplicate create-wallet')
    _try_creating_wallet(client, wallet_name, wallet_value)


def assess_list_wallets(client, expected_wallets):
    log.info('list-wallets testing expected values')
    # Since we can't remove wallets until lp:1663258
    # is fixed, we don't modify the list contents or count
    # Nonetheless, we assert on it for future use
    wallets = _get_wallets(client)
    assert_sorted_equal(wallets, expected_wallets)


def assess_set_wallet(client, wallet_name, wallet_value, wallet_limit):
    """Test set-wallet command"""
    log.info('set-wallet "{}" with value {}, limit {}'.format(wallet_name,
                                                              wallet_value,
                                                              wallet_limit))
    _try_setting_wallet(client, wallet_name, wallet_value)

    # Check some bounds
    # Since walletting is important, and the functional test is cheap,
    # let's test some basic bounds
    log.info('Trying set-wallet with value greater than wallet limit')
    _try_greater_than_limit_wallet(client, wallet_name, wallet_limit)

    log.info('Trying set-wallet with negative value')
    _try_negative_wallet(client, wallet_name)


def assess_show_wallet(client, wallet_name, wallet_value):
    log.info('show-wallet "{}" with value {}'.format(wallet_name,
                                                     wallet_value))

    wallet = json.loads(show_wallet(client, wallet_name))

    # assert wallet value
    if wallet['limit'] != wallet_value:
        raise JujuAssertionError('Wallet limit found {}, expected {}'.format(
            wallet['limit'], wallet_value))

    # assert on usage (0% until we use it)
    if wallet['total']['usage'] != '0%':
        raise JujuAssertionError('Wallet usage found {}, expected {}'.format(
            wallet['total']['usage'], '0%'))


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test wallet commands")
    # Set to false it it's possible to overwrite actual cookie data if someone
    # runs it against an existing environment
    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)


def set_controller_cookie_file(client):
    """Plant pre-generated cookie file to avoid launching Browser.

    Using an existing usso token use 'charm login' to create a .go-cookies file
    (in a tmp HOME). Copy this new cookies file to become the controller cookie
    file.
    """

    with temp_dir() as tmp_home:
        with EnvironmentVariable('HOME', tmp_home):
            move_usso_token_to_juju_home(tmp_home)

            # charm login shouldn't be interactive, fail if it is.
            try:
                command = pexpect.spawn(
                    'charm', ['login'], env={'HOME': tmp_home})
                command.expect(pexpect.EOF)
            except (pexpect).TIMEOUT:
                raise RuntimeError('charm login command was interactive.')

            go_cookie = os.path.join(tmp_home, '.go-cookies')
            controller_cookie_path = os.path.join(
                client.env.juju_home,
                'cookies',
                '{}.json'.format(client.env.controller.name))

            shutil.copyfile(go_cookie, controller_cookie_path)


def move_usso_token_to_juju_home(tmp_home):
    """Move pre-packaged token to juju data dir.

    Move the stored store-usso-token to a tmp juju home dir for charm command
    use.
    """
    source_usso_path = os.path.join(
        os.environ['JUJU_HOME'], 'juju-bot-store-usso-token')
    dest_usso_dir = os.path.join(tmp_home, '.local', 'share', 'juju')
    os.makedirs(dest_usso_dir)
    dest_usso_path = os.path.join(dest_usso_dir, 'store-usso-token')
    shutil.copyfile(source_usso_path, dest_usso_path)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        set_controller_cookie_file(bs_manager.client)
        assess_wallet(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
