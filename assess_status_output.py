#!/usr/bin/env python
from __future__ import print_function

__metaclass__ = type

import logging

from base_asses import get_base_parser
from status import StatusTester
from jujupy import (
    make_client,
    parse_new_state_server_from_error,
    temp_bootstrap_env,
)
from jujuconfig import (
    get_juju_home,
)
from deploy_stack import (
    dump_env_logs,
    get_machine_dns_name,
)
from utility import (
    print_now,
)


def run_complete_status(client, status):
    """run the complete set of tests possible for any StatusParser

    :param client: python juju client.
    :type client: jujupy.EnvJujuClient
    :param status: a BaseStatusParser
    :type status: BaseStatusParser
    """
    status.s.assert_machines_len(2)
    status.s.assert_machines_ids(("0","1"))
    juju_status = client.get_status()
    for name, machine in juju_status.iter_machines(False):
        status.s.assert_machine_agent_state(name, "started")
        status.s.assert_machine_agent_version(name, machine.get("agent-version"))
        status.s.assert_machine_dns_name(name, machine.get("dns-name"))
        status.s.assert_machine_instance_id(name, machine.get("instance-id"))
        status.s.assert_machine_series(name, machine.get("series"))
        status.s.assert_machine_hardware(name, machine.get("hardware"))
        state_server = machine.get("state-server-member-status", None)
        if state_server:
            status.s.assert_machine_member_status(name, "has-vote")

    status.s.assert_service_charm("statusstresser",
                                  "local:trusty/statusstresser-1")
    status.s.assert_service_exposed("statusstresser", False)
    status.s.assert_service_service_status("statusstresser",
                                    {"current": "active",
                                    "message":"called in config-changed hook"})
    status.s.assert_unit_workload_status("statusstresser/0",
                                    {"current": "active",
                                    "message":"called in config-changed hook"})
    status.s.assert_unit_agent_status("statusstresser/0",
                                    {"current": "idle", "message":""})
    status.s.assert_unit_agent_state("statusstresser/0", "started")
    agent_versions = juju_status.get_agent_versions()
    for version in agent_versions:
        for item in agent_versions[version]:
            if not item.isdigit():
                status.s.assert_unit_agent_version(item, version)
    status.s.assert_unit_machine("statusstresser/0", "1")

def run_reduced_status(client, status):
    """run a reduced set of tests for a StatusParser, this is useful for
    status outputs such as Tabular that hold less information.

    :param client: python juju client.
    :type client: jujupy.EnvJujuClient
    :param status: a BaseStatusParser
    :type status: BaseStatusParser
    """
    status.s.assert_machines_len(2)
    status.s.assert_machines_ids(("0","1"))
    juju_status = client.get_status()
    for name, machine in juju_status.iter_machines(False):
        status.s.assert_machine_agent_state(name, "started")
        status.s.assert_machine_agent_version(name, machine.get("agent-version"))
        status.s.assert_machine_dns_name(name, machine.get("dns-name"))
        status.s.assert_machine_instance_id(name, machine.get("instance-id"))
        status.s.assert_machine_series(name, machine.get("series"))
        status.s.assert_machine_hardware(name, machine.get("hardware"))

    status.s.assert_service_charm("statusstresser",
                                  "local:trusty/statusstresser-1")
    status.s.assert_service_exposed("statusstresser", False)
    status.s.assert_service_service_status("statusstresser",
                                    {"current": "active",
                                    "message":""})
    status.s.assert_unit_workload_status("statusstresser/0",
                                    {"current": "active",
                                    "message":"called in config-changed hook"})
    status.s.assert_unit_agent_status("statusstresser/0",
                                    {"current": "idle", "message":""})
    status.s.assert_unit_machine("statusstresser/0", "1")


def test_status_set_on_install(client):
    """Test that status set is proplerly called during install
    and that status is also added to history

    :param client: python juju client.
    :type client: jujupy.EnvJujuClient
    """
    status = StatusTester.from_text(client.get_raw_status(60, "--format=yaml"),
                                    "yaml")
    run_complete_status(client, status)

    status = StatusTester.from_text(client.get_raw_status(60, "--format=json"),
                                    "json")
    run_complete_status(client, status)

    status = StatusTester.from_text(client.get_raw_status(60, "--format=tabular"),
                                    "tabular")

    run_reduced_status(client, status)





def parse_args():
    """Parse all arguments."""
    parser = get_base_parser('Test status outputs')
    return parser.parse_args()


def main():
    args = parse_args()
    log_dir = args.logs

    client = make_client(
        args.juju_path, args.debug, args.env_name, args.temp_env_name)
    client.destroy_environment()
    juju_home = get_juju_home()
    try:
        with temp_bootstrap_env(juju_home, client):
            client.bootstrap()
        bootstrap_host = get_machine_dns_name(client, 0)
        client.get_status(60)
        client.juju("deploy", ('local:trusty/statusstresser',))
        client.wait_for_started()

        test_status_set_on_install(client)

    except Exception as e:
        logging.exception(e)
        try:
            if bootstrap_host is None:
                bootstrap_host = parse_new_state_server_from_error(e)
            dump_env_logs(client, bootstrap_host, log_dir)
        except Exception as e:
            print_now("exception while dumping logs:\n")
            logging.exception(e)
    finally:
        client.destroy_environment()


print(__name__)
if __name__ == '__main__':
    main()
