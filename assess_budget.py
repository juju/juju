#!/usr/bin/env python
"""
This tests the budget commands utilized for commercial charm billing.
These commands are linked to a ubuntu sso account, and as such, require the
user account to be setup before test execution (including authentication).
You can use charm login to do this, or let juju authenticate with a browser.
"""

from __future__ import print_function

import argparse
import json
import logging
from random import randint
import subprocess
import sys


from jujupy import (
    client_from_config,
)
from utility import (
    add_basic_testing_arguments,
    JujuAssertionError,
    configure_logging,
)

__metaclass__ = type


log = logging.getLogger("assess_budget")


def _get_new_budget_limit(client):
    """Return availible limit for new budget"""
    budgets = json.loads(list_budgets(client))
    limit = int(budgets['total']['limit'])
    credit = int(budgets['credit'])
    log.debug('Found credit limit {}, currently used {}'.format(
        credit, limit))
    return credit - limit


def _get_budgets(client):
    return json.loads(list_budgets(client))['budgets']


def _set_budget_value_expectations(expected_budgets, name, value):
    # Update our expectations accordingly
    for budget in expected_budgets:
        if budget['budget'] == name:
            # For now, we assume we aren't spending down the budget
            budget['limit'] = value
            budget['unallocated'] = value
            # .00 is appended to availible for some reason
            budget['available'] = '{:.2f}'.format(float(value))
            log.info('Expected budget updated: "{}" to {}'.format(name, value))


def _try_setting_budget(client, name, value):
    try:
        output = set_budget(client, name, value)
    except subprocess.CalledProcessError as e:
        output = [e.output, getattr(e, 'stderr', '')]
        raise JujuAssertionError('Could not set budget {}'.format(output))

    if 'budget limit updated' not in output:
        raise JujuAssertionError('Error calling set-wallet {}'.format(output))


def _try_creating_budget(client, name, value):
    try:
        create_budget(client, name, value)
        log.info('Created new budget "{}" with value {}'.format(name,
                                                                value))
    except subprocess.CalledProcessError as e:
        output = [e.output, getattr(e, 'stderr', '')]
        if any('already exists' in message for message in output):
            log.info('Reusing budget "{}" with value {}'.format(name, value))
            pass  # this will be a failure once lp:1663258 is fixed
        else:
            raise JujuAssertionError(
                'Error testing create-budget: {}'.format(output))
    except:
        raise JujuAssertionError('Added duplicate budget')


def _try_greater_than_limit_budget(client, name, limit):
    error_strings = {
        'pass': 'exceed the credit limit',
        'unknown': 'Error testing budget greater than credit limit',
        'fail': 'Credit limit exceeded'
    }
    over_limit_value = str(limit + randint(1, 100))
    assert_set_budget(client, name, over_limit_value, error_strings)


def _try_negative_budget(client, name):
    error_strings = {
        'pass': 'Could not set budget',
        'unknown': 'Error testing negative budget',
        'fail': 'Negative budget allowed'
    }
    negative_budget_value = str(randint(-1000, -1))
    assert_set_budget(client, name, negative_budget_value, error_strings)


def assert_sorted_equal(found, expected):
    found = sorted(found)
    expected = sorted(expected)
    if found != expected:
        raise JujuAssertionError(
            'Found: {}\nExpected: {}'.format(found, expected))


def assert_set_budget(client, name, limit, error_strings):
    try:
        _try_setting_budget(client, name, limit)
    except JujuAssertionError as e:
        if error_strings['pass'] not in e.message:
            raise JujuAssertionError(
                '{}: {}'.format(error_strings['unknown'], e))
    else:
        raise JujuAssertionError(error_strings['fail'])


def create_budget(client, name, value):
    """Create a budget"""
    return client.get_juju_output('create-budget', name, value,
                                  include_e=False)


def list_budgets(client):
    """Return defined budgets as json."""
    return client.get_juju_output('list-wallets', '--format', 'json',
                                  include_e=False)


def set_budget(client, name, value):
    """Change an existing budgets allocation."""
    return client.get_juju_output('set-wallet', name, value, include_e=False)


def show_budget(client, name):
    """Return specified budget as json."""
    return client.get_juju_output('show-wallet', name, '--format', 'json',
                                  include_e=False)


def assess_budget(client):
    # Since we can't remove budgets until lp:1663258
    # is fixed, we avoid creating new random budgets and hardcode.
    # We also, zero out the previous budget
    budget_name = 'test'
    _try_setting_budget(client, budget_name, '0')

    budget_limit = _get_new_budget_limit(client)
    assess_budget_limit(budget_limit)

    expected_budgets = _get_budgets(client)
    budget_value = str(randint(1, budget_limit / 2))
    assess_create_budget(client, budget_name, budget_value, budget_limit)

    budget_value = str(randint(budget_limit / 2 + 1, budget_limit))
    assess_set_budget(client, budget_name, budget_value, budget_limit)
    assess_show_budget(client, budget_name, budget_value)

    _set_budget_value_expectations(expected_budgets, budget_name, budget_value)
    assess_list_budgets(client, expected_budgets)


def assess_budget_limit(budget_limit):
    log.info('Assessing budget limit {}'.format(budget_limit))

    if budget_limit < 0:
        raise JujuAssertionError(
            'Negative Budget Limit {}'.format(budget_limit))


def assess_create_budget(client, budget_name, budget_value, budget_limit):
    """Test create-budget command"""
    log.info('create-budget "{}" with value {}, limit {}'.format(budget_name,
                                                                 budget_value,
                                                                 budget_limit))

    # Do this twice, to ensure budget exists and we can check for
    # duplicate message. Ideally, once lp:1663258 is fixed, we will
    # assert on initial budget creation as well.
    _try_creating_budget(client, budget_name, budget_value)

    log.info('Trying duplicate create-budget')
    _try_creating_budget(client, budget_name, budget_value)


def assess_list_budgets(client, expected_budgets):
    log.info('list-wallets testing expected values')
    # Since we can't remove budgets until lp:1663258
    # is fixed, we don't modify the list contents or count
    # Nonetheless, we assert on it for future use
    budgets = _get_budgets(client)
    assert_sorted_equal(budgets, expected_budgets)


def assess_set_budget(client, budget_name, budget_value, budget_limit):
    """Test set-wallet command"""
    log.info('set-wallet "{}" with value {}, limit {}'.format(budget_name,
                                                              budget_value,
                                                              budget_limit))
    _try_setting_budget(client, budget_name, budget_value)

    # Check some bounds
    # Since budgetting is important, and the functional test is cheap,
    # let's test some basic bounds
    log.info('Trying set-wallet with value greater than budget limit')
    _try_greater_than_limit_budget(client, budget_name, budget_limit)

    log.info('Trying set-wallet with negative value')
    _try_negative_budget(client, budget_name)


def assess_show_budget(client, budget_name, budget_value):
    log.info('show-wallet "{}" with value {}'.format(budget_name,
                                                     budget_value))

    budget = json.loads(show_budget(client, budget_name))

    # assert budget value
    if budget['limit'] != budget_value:
        raise JujuAssertionError('Budget limit found {}, expected {}'.format(
            budget['limit'], budget_value))

    # assert on usage (0% until we use it)
    if budget['total']['usage'] != '0%':
        raise JujuAssertionError('Budget usage found {}, expected {}'.format(
            budget['total']['usage'], '0%'))


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test budget commands")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    client = client_from_config(args.env, args.juju_bin, False)
    assess_budget(client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
