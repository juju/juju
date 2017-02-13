#!/usr/bin/env python
"""TODO: add rough description of what is assessed in this module."""

from __future__ import print_function

import argparse
import logging
import sys

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


def assess_budget(client):
    # Deploy charms, there are several under ./repository
    # client.deploy('local:trusty/my-charm')
    # Wait for the deployment to finish.
    #client.wait_for_started()
    log.info("TODO: Add log line about any test")
    # TODO: Add specific functional testing actions here.

    # juju list-budgets
    # assert no budgets
    client.list_budgets()
    
    client.show_budget('test')

    # juju create-budget test 10
    # create a new budget
    budget_name = 'test'
    budget_value = '10'
    client.create_budget(budget_name, budget_value)
    
    # juju create-budget test 10
    # create same budget
    # assert ERROR failed to create the budget: budget "nskaggs/test" already exists
    client.create_budget(budget_name, budget_value)
    assert

    # juju set-budget test 2
    # assert juju list-budgets shows test budget value is now 2

    # juju show-budget test
    # assert on usage (0% until we use it)


    



def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="TODO: script info")
    # TODO: Add additional positional arguments.
    add_basic_testing_arguments(parser)
    # TODO: Add additional optional arguments.
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
            username = user.name
        if controller_name is None:
            controller_name = '{}_controller'.format(username)

        model = self.env.environment
        token = self.add_user_perms(username, models=model,
                                    permissions=user.permissions)
        user_client = self.create_cloned_environment(juju_home,
                                                     controller_name,
                                                     username)
                                                     
    client = client_from_config(args.env, args.juju_bin, False)
    client.env.load_yaml()
    assess_budget(client)
    #bs_manager = BootstrapManager.from_args(args)
    #with bs_manager.booted_context(args.upload_tools):
    #    assess_budget(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
