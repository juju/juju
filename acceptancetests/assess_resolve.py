#!/usr/bin/env python3
""" Ensure resolve command resolves hook issues.

This test covers the resolve command (and the option --no-retry)

The test:
  - Uses a simple custom charm that instruments failure in the install hook
  - Ensures that 'resolve' retries the failed install hook and that the
    followup 'start' hook is fired.
  - Ensures that 'resolve --no-retry' does NOT re-run the failed install hook
    and instead goes ahead and runs the follow up 'started' hook.
"""

from __future__ import print_function

import argparse
import logging
import sys

from deploy_stack import (
    BootstrapManager,
    )
from jujupy.models import (
    temporary_model,
    )
from jujupy.wait_condition import (
    UnitInstallCondition,
    )
from jujucharm import local_charm_path
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    )


__metaclass__ = type


log = logging.getLogger('assess_resolve')


class ResolveCharmMessage:
    """Workload active message differs if the install hook completed."""
    ACTIVE_NO_INSTALL_HOOK = 'No install hook'
    ACTIVE_INSTALL_HOOK = 'Install hook succeeded'
    INSTALL_FAIL = 'hook failed: "install"'


def assess_resolve(client, local_charm='simple-resolve'):
    local_resolve_charm = local_charm_path(
        charm=local_charm, juju_ver=client.version)
    ensure_retry_does_not_rerun_failed_hook(client, local_resolve_charm)


def ensure_retry_does_not_rerun_failed_hook(client, resolve_charm):
        with temporary_model(client, "no-retry") as temp_client:
            unit_name = 'simple-resolve/0'
            temp_client.deploy(resolve_charm)
            temp_client.wait_for(UnitInstallError(unit_name))
            temp_client.juju('resolve', ('--no-retry', unit_name))
            # simple-resolve start hook sets a message when active to indicate
            # it ran and if the install hook ran successfully or not.
            # Here we make sure it's active and no install hook success.
            temp_client.wait_for(
                UnitInstallActive(
                    unit_name, ResolveCharmMessage.ACTIVE_NO_INSTALL_HOOK))


class UnitInstallError(UnitInstallCondition):
    """Wait until `unit` is in error state with message status `message`.

    Useful to determine when a unit is in an expected error state (the message
    check allows further confirmation the error reason is the one we're looking
    for)
    """
    def __init__(self, unit, *args, **kwargs):
        super(UnitInstallError, self).__init__(
            unit, 'error', ResolveCharmMessage.INSTALL_FAIL, *args, **kwargs)


class UnitInstallActive(UnitInstallCondition):
    """Wait until `unit` is in active state with message status `message`

    Useful to determine when a unit is active for a specific reason.
    """

    def __init__(self, unit, message, *args, **kwargs):
        super(UnitInstallActive, self).__init__(
            unit, 'active', message, *args, **kwargs)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Ensure the resolve command operates to spec.')
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_resolve(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
