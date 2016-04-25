#!/usr/bin/python
from __future__ import print_function

from argparse import ArgumentParser
from status import StatusTester
from deploy_stack import (
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    local_charm_path,
)


__metaclass__ = type


def run_complete_status(client, status):
    """run the complete set of tests possible for any StatusParser.

    :param client: python juju client.
    :type client: jujupy.EnvJujuClient
    :param status: a BaseStatusParser
    :type status: BaseStatusParser
    """
    status.s.assert_machines_len(2)
    status.s.assert_machines_ids(("0", "1"))
    juju_status = client.get_status()
    for name, machine in juju_status.iter_machines(False):
        status.s.assert_machine_agent_state(name, "started")
        status.s.assert_machine_agent_version(name,
                                              machine.get("agent-version"))
        status.s.assert_machine_dns_name(name, machine.get("dns-name"))
        status.s.assert_machine_instance_id(name,
                                            machine.get("instance-id"))
        status.s.assert_machine_series(name, machine.get("series"))
        status.s.assert_machine_hardware(name, machine.get("hardware"))
        state_server = client.get_controller_member_status(machine)
        if state_server:
            status.s.assert_machine_member_status(name, "has-vote")

    status.s.assert_service_charm("statusstresser",
                                  "local:trusty/statusstresser-1")
    status.s.assert_service_exposed("statusstresser", False)
    status.s.assert_service_service_status("statusstresser",
                                           {"current": "active",
                                            "message": "called in "
                                            "config-changed hook"})
    status.s.assert_unit_workload_status("statusstresser/0",
                                         {"current": "active",
                                          "message": "called in "
                                          "config-changed hook"})
    status.s.assert_unit_agent_status("statusstresser/0",
                                      {"current": "idle", "message": ""})
    status.s.assert_unit_agent_state("statusstresser/0", "started")
    agent_versions = juju_status.get_agent_versions()
    for version in agent_versions:
        for item in agent_versions[version]:
            if not item.isdigit():
                status.s.assert_unit_agent_version(item, version)
    status.s.assert_unit_machine("statusstresser/0", "1")


def run_reduced_status(client, status):
    """run a subset of the status asserts.

    run a reduced set of tests for a StatusParser, this is useful for
    status outputs such as Tabular that hold less information.

    :param client: python juju client.
    :type client: jujupy.EnvJujuClient
    :param status: a BaseStatusParser
    :type status: BaseStatusParser
    """
    status.s.assert_machines_len(2)
    status.s.assert_machines_ids(("0", "1"))
    juju_status = client.get_status()
    for name, machine in juju_status.iter_machines(False):
        status.s.assert_machine_agent_state(name, "started")
        status.s.assert_machine_agent_version(name,
                                              machine.get("agent-version"))
        status.s.assert_machine_dns_name(name, machine.get("dns-name"))
        status.s.assert_machine_instance_id(name, machine.get("instance-id"))
        status.s.assert_machine_series(name, machine.get("series"))
        status.s.assert_machine_hardware(name, machine.get("hardware"))

    status.s.assert_service_charm("statusstresser",
                                  "local:trusty/statusstresser-1")
    status.s.assert_service_exposed("statusstresser", False)
    status.s.assert_service_service_status("statusstresser",
                                           {"current": "active",
                                            "message": ""})
    status.s.assert_unit_workload_status("statusstresser/0",
                                         {"current": "active",
                                          "message": "called in "
                                          "config-changed hook"})
    status.s.assert_unit_agent_status("statusstresser/0",
                                      {"current": "idle", "message": ""})
    status.s.assert_unit_machine("statusstresser/0", "1")


def test_status_set_on_install(client):
    """Test the status after install.

    Test that status set is proplerly called during install and
    that all formats are returning proper information.

    :param client: python juju client.
    :type client: jujupy.EnvJujuClient
    """
    status = StatusTester.from_text(client.get_status(60, True, False,
                                                      "--format=yaml"),
                                    "yaml")
    run_complete_status(client, status)
    status = StatusTester.from_text(client.get_status(60, True, False,
                                                      "--format=json"),
                                    "json")
    run_complete_status(client, status)
    status = StatusTester.from_text(client.get_status(60, True, False,
                                                      "--format=tabular"),
                                    "tabular")
    run_reduced_status(client, status)


def parse_args():
    """Parse all arguments."""
    parser = ArgumentParser('Test status outputs')
    add_basic_testing_arguments(parser)
    return parser.parse_args()


def main():
    args = parse_args()
    series = args.series
    if series is None:
        args.series = 'precise'
    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    with bs_manager.booted_context(args.upload_tools):
        client.get_status(60)
        charm = local_charm_path(
            charm='statusstresser', juju_ver=client.version, series='trusty')
        client.deploy(charm)
        client.wait_for_started()
        test_status_set_on_install(client)


if __name__ == '__main__':
    main()
