#!/usr/bin/env python
"""This will test the budget commands utilized for commercial charm billing.
These commands are linked to a ubuntu one account, and as such, require the
user account to be setup before test execution (including authentication)."""

from __future__ import print_function

import argparse
import logging
import sys
import subprocess
from random import randint

from deploy_stack import (
    BootstrapManager,
    )
from utility import (
    add_basic_testing_arguments,
    JujuAssertionError,
    configure_logging,
    )
from jujupy import (
    client_from_config,
    )


__metaclass__ = type


log = logging.getLogger("assess_budget")


def list_budgets(client):
    """Return defined budgets as json."""
    return client.get_juju_output('list-budgets', '--format', 'json', include_e=False)

def show_budget(client, name):
    """Return specified budget as json."""
    return client.get_juju_output('show-budget', name, '--format', 'json')

def create_budget(client, name, value):
    """Create a budget."""
    return client.get_juju_output('create-budget', name, value, include_e=False)

def set_budget(client, name):
    """Change an existing budgets allocation."""
    return client.get_juju_output('set-budget', name)

def assess_create_budget(client, budget_name, budget_value):
    """Test create budget command"""
    # Do this twice, to ensure budget exists and we can check for
    # duplicate message. Ideally, once lp:1663258 is fixed, we will
    # assert on initial budget creation as well. For this reason, the
    # code is also duplicated as it should be a failure if found duplicate.
    # Assert on ERROR failed to create the budget: budget "*" already exists
    try:
        create_budget(client, budget_name, budget_value)
        log.info('Created new budget {} with value {}'.format(budget_name,
                                                             budget_value))
    except subprocess.CalledProcessError as e:
        output = [e.output, e.stderr]
        if any('already exists' in message for message in output):
            pass
            # this should be a failure once lp:1663258 is fixed
        else:
            raise JujuAssertionError(
                'Duplicate budget not allowed for unknown reason {}'.format(
                output))
    else:
        raise JujuAssertionError('Added duplicate budget')
        
    try:
        create_budget(client, budget_name, budget_value)
    except subprocess.CalledProcessError as e:
        output = [e.output, e.stderr]
        if any('already exists' in message for message in output):
            pass
        else:
            raise JujuAssertionError(
                'Duplicate budget not allowed for unknown reason {}'.format(
                output))
    else:
        raise JujuAssertionError('Added duplicate budget')

def assess_set_budget(client, budget_name, budget_value):
    """Test set, show, and list budget commands"""    
    set_budget(client, budget_name, budget_value)
    # juju list-budgets
    # assert budget value 
    list_budgets(client)

    show_budget(client, budget_name)
    # assert budget value
    # assert on usage (0% until we use it)

def assess_budget(client):
    # Deploy charms, there are several under ./repository
    # client.deploy('local:trusty/my-charm')
    # Wait for the deployment to finish.
    #client.wait_for_started()

    # Since we can't remove budgets until lp:1663258
    # is fixed, we avoid creating new random budgets and hardcode.
    budget_name = 'test'

    assess_create_budget(client, budget_name, randint(1,1000))
    assess_set_budget(client, budget_name, randint(1001,10000))

def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test budget commands")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)                                                     
    client = client_from_config(args.env, args.juju_bin, False)
    #client.env.load_yaml()
    assess_budget(client)
    #bs_manager = BootstrapManager.from_args(args)
    #with bs_manager.booted_context(args.upload_tools):
    #    assess_budget(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
