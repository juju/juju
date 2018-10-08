#!/usr/bin/env python
""" Assess updating the series of existing and future juju units.

Bootstrap juju and check that juju handles the series updates of applications
appropriately and assists getting the juju agents back up and running if the
service changes.

  - Bootstraps a controller
  - Deploy two units of a charm using trusty as a series
  - Update each to Xenial, LTS version, reboot one but not the other.
  - Run jujud-updateseries command twice on rebooted unit, using
    --start-agents the second time, agents should start.
  - Run jujud-updateseries command twice on non-rebooted unit, using
    --start-agents the second time, agents won't be running.
  - Reboot the unit which hadn't be rebooted already, agents should start.
  - Run juju update-series to update the series of both machine in juju status
  - Run juju update-series to update the series the charm uses to deploy.
  - Deploy a 3rd unit of the charm, ensure is using the new series.
"""

from __future__ import print_function

import argparse
import logging
import subprocess
import sys

from deploy_stack import (
    BootstrapManager,
)
from jujucharm import (
    local_charm_path,
)
from textwrap import dedent
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    wait_for_port,
)

__metaclass__ = type

log = logging.getLogger("assess_upgrade-series")
charm_app = 'dummy-source'

expectedStartAgents = dedent("""\
started jujud-unit-{}-{} service
started jujud-machine-{} service
all agents successfully restarted
""")


def assess_juju_upgrade_series_prepare(client, args):
    """ Tests juju-updateseries --to-series <series> --from-series <series>

    Using the origin trusty unit, run do-release-upgrade
    Run juju-updateseries on unit
    Run juju-updateseries on unit --start-agents, it should fail
    Reboot the unit, verify agents start.

    Add a new trusty unit,
    run do-release-upgrade and reboot
    Run juju-updateseries on unit
    Run juju-updateseries on unit with --start-agents
    Verify agents start.
    """
    target_machine = '0'
    upgrade_series_prepare(client, target_machine, args.to_series, True)
    do_release_upgrade(client, target_machine)
    reboot_machine(client, target_machine)
    upgrade_series_complete(client, target_machine)
#    unit_num = client.get_status().get_service_unit_count(charm_app)
#    client.juju('add-unit', (charm_app))
#    client.wait_for_started()
#    upgrade_release(
#        client, unit_num, args.from_series, args.to_series, 'before')

def upgrade_series_prepare(client, machine, series, agree=False):
    """ Run juju update-series with given arguments

    :param client: Juju client
    :param machine: machine number to upgrade
    :param series: series to upgrade to
    :param agree: premptively agree to all changes before prompted
    :return:
    """
    args = (machine, series)
    if agree:
        args += ('--agree',)
    client.juju('upgrade-series prepare', args)


def upgrade_series_complete(client, machine):
    """ Run juju update-series with given arguments

    :param client: Juju client
    :param machine: machine number to upgrade
    :return:
    """
    args = (machine)
    client.juju('upgrade-series complete', args)

def do_release_upgrade(client, machine):
    """ Update the series of the given unit.

    :param client: Juju client
    :param machine: machine number to use
    :param from_series: original series of unit
    :param to_series: series to update the unit to
    :param reboot: 'before' or 'after' juju-updateseries is called
    :return: None
    :raises JujuAssertionError: if juju-updateseries doesn't act as expected.
    :raises AssertionError: non juju failures in the process.
    """

    try:
        output = client.get_juju_output(
            'ssh', machine, 'sudo do-release-upgrade -f '
            'DistUpgradeViewNonInteractive', timeout=3600)
    except subprocess.CalledProcessError as e:
        raise AssertionError(
            "do-release-upgrade failed on {}: {}".format(unit, e))

    log.info("do-release-upgrade response: ".format(output))

def reboot_machine(client, machine):
    """ Reboot a unit, wait for the agents to restart, or if not agents
    restarting is not expected, wait for the ssh port to be available.

    :param client: Juju client
    :param unit: unit to reboot
    :param expect_agents: bool, verify reboot via waiting for agents to
    restart on unit.  If not, verify by waiting for port 22 being available.
    :return: None
    """
    try:
        log.info("Restarting: {}".format(machine))
        cmd = build_ssh_cmd(client, machine, 'sudo shutdown -r now && exit')
        output = subprocess.check_output(cmd, stderr=subprocess.STDOUT)
        log.info("Restarting machine output: {}\n".format(output))
    except subprocess.CalledProcessError as e:
        # There is a failure logged with run with expect_agents False,
        # though the reboot still happens???
        logging.info(
            "Error running shutdown:\nstdout: %s\nstderr: %s",
            e.output, getattr(e, 'stderr', None))

    log.info("wait_for_started()")
    client.wait_for_started()
    # else:
    #     # for cases where machine agents are not expected to be
    #     # running after reboot, wait for the ssh port to be available
    #     # instead.
    #     status = client.get_status()
    #     unit_status = dict(status.iter_units())
    #     m = unit_status[unit]['machine']
    #     machine_status = dict(status.iter_machines())
    #     hostname = machine_status[m]['dns-name']
    #     log.info("wait_for_port({}:22)".format(hostname))
    #     wait_for_port(hostname, 22, timeout=240)


    #if reboot == 'before':
    #    reboot_unit(client, unit, False)

    # NOTE: attempts to use calls such as client.juju() or remote.ssh() from
    # this point on return timeouts.  The reboot above timesout, though a
    # reboot does occur.  Subsequent attempts to access the unit time out.
    # The cause is unknown, but suspected to be in the CI packages as
    # subprocess works and is used from here on out.

    # assert_correct_series(client, unit, to_series)

    # cmd = build_ssh_cmd(
    #         client, unit, 'sudo juju-updateseries --to-series '
    #         '{} --from-series {}'.format(to_series, from_series))
    # try:
    #     output = subprocess.check_output(cmd, stderr=subprocess.STDOUT)
    # except subprocess.CalledProcessError as e:
    #     raise JujuAssertionError(
    #         "error running juju-updateseries on {}: {}".format(unit, e))

    # log.info(
    #     "juju-updateseries ... succeeded:\n{}".format(output))

    # if "successfully copied tools and relinked agent tools" not in output:
    #     raise JujuAssertionError("failure in juju-updateseries")

    # cmd = build_ssh_cmd(
    #         client, unit, 'sudo juju-updateseries --to-series '
    #         '{} --from-series {} --start-agents'.format(
    #         to_series, from_series))
    # try:
    #     output = subprocess.check_output(cmd, stderr=subprocess.STDOUT)
    # except subprocess.CalledProcessError as e:
    #     if reboot in 'before':
    #         if expectedStartAgents.format(charm_app, unit_num, unit_num) \
    #                 not in output:
    #             raise JujuAssertionError("above output incorrect")
    #     elif reboot in 'after':
    #         # if the unit has NOT been rebooted before calling updateseries
    #         # with --start-agents, and ERROR will be returned, but that's
    #         # expected, look for it here
    #         if 'systemd is not fully running, please reboot to start agents'\
    #                         not in e.output:
    #             raise JujuAssertionError(
    #                 "error running juju-updateseries on {}: {}".format(
    #                 unit, e))
    #     else:
    #         log.critical("test failure: reboot no longer before or after")
    #         raise

    # log.info('sudo juju-updateseries ... --start-agents: {}'.format(output))

    # if reboot == 'after':
    #     reboot_unit(client, unit, True)
    # else:
    #     client.wait_for_started()

    # log.info(
    #     "juju-updateseries .... --start-agents, reboot "
    #     "{} command, succeeded".format(reboot))


def build_ssh_cmd(client, machine, command):
    """ build_ssh_cmd is a helper method taking pieces from Client and Remote

    :param client: Juju client
    :param unit: unit to ssh to
    :param command: command it be run via ssh
    :return: completed command to which can be run
    """
    ssh_opts = [
        "-o", "User ubuntu",
        "-o", "UserKnownHostsFile /dev/null",
        "-o", "StrictHostKeyChecking no",
        "-o", "PasswordAuthentication no",
    ]

    status = client.get_status()
    machine_status = status.get_machine(machine)
    cmd = ["ssh"] + ssh_opts + [machine_status['public-address']] + [command]
    return cmd


def assert_correct_series(client, machine, expected):
    """ Verify the unit is now running the expected series.

    :param client: Juju client
    :param unit: unit to find series of
    :param expected: series the unit is expected to have
    :return: None
    """
    cmd = build_ssh_cmd(client, unit, 'lsb_release -c')
    try:
        lsb_release = subprocess.check_output(cmd, stderr=subprocess.STDOUT)
    except subprocess.CalledProcessError as e:
        raise AssertionError(
            "lsb_release -c failed on {}: {}".format(unit, e))

    if expected not in lsb_release:
        raise AssertionError(
            "Series from {} doesn't match expected: {}".format(unit, expected))


def assess_juju_updateseries_machine(client, args):
    """ Tests juju update-series <machine> <series>

    Update the machine series
    Verify with juju status
    Return machine's series to original
    """
    update_machine_series_verify(client, args.from_series)
    update_machine_series_verify(client, args.to_series)


def update_machine_series_verify(client, series):
    """ Run juju update-series <machine> <series>, then check
    juju status returns expected data.  Always runs on machine 0.

    :param client: Juju client
    :param series: series to change to
    :return: None
    :raises JujuAssertionError: if series is not appropriately updated.
    """
    update_series(client, "0", series)
    status = client.get_status()
    machine_info = dict(status.iter_machines())

    machine_series = machine_info["0"]['series']
    log.info(
        "unit series {}, machine series {}".format(series, machine_series))

    if series not in machine_series:
        raise JujuAssertionError(
            "Series in juju status for machine-0 is not {}, per juju".format(
                series))

    log.info("juju update-series 0 {} succeeded".format(series))


def assess_juju_updateseries_application(client, args):
    """ Tests juju update-series <application> <series>

    Update the application series
    Verify with juju status
    Deploy a new unit, does the new unit have the expected series
    """
    update_application_series_verify(client, args.to_series)
    update_application_series_verify(client, args.from_series)


def update_application_series_verify(client, series):
    """  Run juju update-series <application> <series>, then check
    juju status returns expected data, and a new unit deploys with
    expected series.  Application charm_app variable will be used.

    :param client: Juju client
    :param series: series to change to
    :return: None
    :raises JujuAssertionError: if series is not appropriately updated.
    """
    update_series(client, charm_app, series)
    unit_num = client.get_status().get_service_unit_count(charm_app)
    client.juju('add-unit', (charm_app))
    client.wait_for_started()

    unit = "{}/{}".format(charm_app, unit_num)

    status = client.get_status()
    app_info = status.get_applications()[charm_app]
    if series not in app_info['series']:
        raise JujuAssertionError(
            "Application series did not change to {}, per juju".format(series))

    unit_machine = status.get_unit(unit)['machine']
    machine_info = dict(status.iter_machines())
    machine_series = machine_info[unit_machine]['series']

    if series not in machine_series:
        raise JujuAssertionError(
            "New unit's series on machine-{} is not {}, per juju".format(
                unit_machine['machine'], series))

    try:
        lsb_release = client.run(["lsb_release -c"], units=[unit])
    except subprocess.CalledProcessError as e:
        log.warning("Could not get series of {} from machine".format(unit))

    if series not in lsb_release[0]['Stdout']:
        raise JujuAssertionError(
            "Series from {} doesn't match expected: {}".format(unit, series))
    log.info("juju update-series {} {} succeeded".format(charm_app, series))

def setup(client, start_series):
    """ Deploy charms, there are several under ./repository """
    charm_source = local_charm_path(
        charm=charm_app, juju_ver=client.version)
    _, deploy_complete = client.deploy(charm_source, series=start_series)
    log.info("Deployed {} with {}".format(charm_app, start_series))
    # Wait for the deployment to finish.
    client.wait_for(deploy_complete)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test juju update series.")
    add_basic_testing_arguments(parser)
    parser.add_argument('--from-series', default='xenial', dest='from_series',
                        help='Series to start machine and units with')
    parser.add_argument('--to-series', default='bionic', dest='to_series',
                        help='Series to upgrade machine and units to')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        setup(bs_manager.client, args.from_series)
        assess_juju_upgrade_series_prepare(bs_manager.client, args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
