#! /usr/bin/env python2

import argparse
import logging
import typing
import yaml

from deploy_stack import BootstrapManager
from utility import add_basic_testing_arguments, add_arg_juju_bin, configure_logging

Optional = typing.Optional
log = logging.getLogger("remove_machine")

def list_machines(client):
    machines = yaml.safe_load(
        client.get_juju_output(
            'list-machines', '--format=yaml'))['machines']
    return machines

def assess_remove_machine(client, series):
    """
    assess_remove_machine checks to see whether
    juju can (add and) remove machines.

    :param client: The jujupy client
    :type client: jujupy.Client
    :param series: The charm series under test
    :type series: Optional[str]
    :return: None
    """
    log.info("start assess_remove_machine")
    client.juju("add-machine", tuple())
    client.wait_for_started()
    machines = list_machines(client)
    assert len(machines) == 1
    machine_ids = list(machines.keys())
    cond = client.remove_machine(machine_ids)
    client.wait_for(cond)
    assert len(list_machines(client)) == 0
    log.info("PASS assess_remove_machine")

def assess_force_remove_machine(client, series):
    """
    assess_remove_machine checks to see whether
    juju can (add and) remove machines that are
    stuck in the pending state.

    :param client: The jujupy client
    :type client: jujupy.Client
    :param series: The charm series under test
    :type series: Optional[str]
    :return: None
    """
    log.info("start assess_force_remove_machine")
    for _ in range(3):
        client.juju("add-machine", ("--constraints", "mem=9999P"))
    machines = list_machines(client)
    assert len(machines) == 3
    machine_ids = list(machines.keys())
    cond = client.remove_machine(machine_ids, force=True)
    client.wait_for(cond)
    assert len(list_machines(client)) == 0
    log.info("PASS assess_force_remove_machine")



def setup(logger=log):
    parser = argparse.ArgumentParser(description="Test juju remove-machine")
    add_basic_testing_arguments(parser)
    args = parser.parse_args()
    configure_logging(args.verbose, logger)
    return args


def main():
    args = setup()
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_remove_machine(bs_manager.client, bs_manager.series)
        assess_force_remove_machine(bs_manager.client, bs_manager.series)

if __name__ == "__main__":
    main()
