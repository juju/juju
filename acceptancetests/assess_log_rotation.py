#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
from datetime import datetime
import re

import yaml

from deploy_stack import (
    boot_context,
    update_env,
    )
from jujucharm import (
    local_charm_path,
    )
from jujupy import (
    client_from_config,
    jes_home_path,
    )
from utility import (
    add_basic_testing_arguments,
    )


__metaclass__ = type


FILL_TIMEOUT = '8m'


class LogRotateError(Exception):

    ''' LogRotate test Exception base class. '''

    def __init__(self, message):
        super(LogRotateError, self).__init__(message)


def assess_debug_log(client, timeout=180, lines=100):
    """After doing log rotation, we should be able to see debug-log output."""
    out = client.get_juju_output("debug-log", "--lines={}".format(lines),
                                 "--limit={}".format(lines), timeout=timeout)
    content = out.splitlines()
    if len(content) != lines:
        raise LogRotateError("We expected {} lines of output, got {}".format(
            lines, len(content)))


def test_unit_rotation(client):
    """Tests unit log rotation."""
    # TODO: as part of testing that when a unit sending lots of logs triggers
    # unit log rotation, we should also test that all-machines.log and future
    # logsink.log get rotated.
    # It would also be possible to test that the logs database doesn't grow too
    # large.
    test_rotation(client,
                  "/var/log/juju/unit-fill-logs-0.log",
                  "unit-fill-logs-0",
                  "fill-unit",
                  "unit-size",
                  "megs=300")
    # TODO: either call assess_debug_log here or add a new assess entry for it.


def assess_machine_rotation(client):
    """Tests machine log rotation."""
    status = client.wait_for_started()
    machine_id = status.get_unit('fill-logs/0')['machine']
    test_rotation(client,
                  "/var/log/juju/machine-{}.log".format(machine_id),
                  "machine-{}".format(machine_id),
                  "fill-machine",
                  "machine-size", "megs=300", "machine={}".format(machine_id))


def test_rotation(client, logfile, prefix, fill_action, size_action, *args):
    """A reusable help for testing log rotation.log

    Deploys the fill-logs charm and uses it to fill the machine or unit log and
    test that the logs roll over correctly.
    """

    # the rotation point should be 300 megs, so let's make sure we hit that.hit
    # we'll obviously already have some data in the logs, so adding exactly
    # 300megs should do the trick.

    # we run do_fetch here so that we wait for fill-logs to finish.
    client.action_do_fetch("fill-logs/0", fill_action, FILL_TIMEOUT, *args)
    out = client.action_do_fetch("fill-logs/0", size_action)
    action_output = yaml.safe_load(out)

    # Now we should have one primary log file, and one backup log file.
    # The backup should be approximately 300 megs.
    # The primary should be below 300.

    check_log0(logfile, action_output)
    check_expected_backup("log1", prefix, action_output)

    # we should only have one backup, not two.
    check_for_extra_backup("log2", action_output)

    # do it all again, this should generate a second backup.

    client.action_do_fetch("fill-logs/0", fill_action, FILL_TIMEOUT, *args)
    out = client.action_do_fetch("fill-logs/0", size_action)
    action_output = yaml.safe_load(out)

    # we should have two backups.
    check_log0(logfile, action_output)
    check_expected_backup("log1", prefix, action_output)
    check_expected_backup("log2", prefix, action_output)

    check_for_extra_backup("log3", action_output)

    # one more time... we should still only have 2 backups and primary

    client.action_do_fetch("fill-logs/0", fill_action, FILL_TIMEOUT, *args)
    out = client.action_do_fetch("fill-logs/0", size_action)
    action_output = yaml.safe_load(out)

    check_log0(logfile, action_output)
    check_expected_backup("log1", prefix, action_output)
    check_expected_backup("log2", prefix, action_output)

    # we should have two backups.
    check_for_extra_backup("log3", action_output)


def check_for_extra_backup(logname, action_output):
    """Check that there are no extra backup files left behind."""
    log = action_output["results"]["result-map"].get(logname)
    if log is None:
        # this is correct
        return
    # log exists.
    name = log.get("name")
    if name is None:
        name = "(no name)"
    raise LogRotateError("Extra backup log after rotation: " + name)


def check_expected_backup(key, logprefix, action_output):
    """Check that there the expected backup files exists and is close to 300MB.
    """
    log = action_output["results"]["result-map"].get(key)
    if log is None:
        raise LogRotateError(
            "Missing backup log '{}' after rotation.".format(key))

    backup_pattern = "/var/log/juju/%s-(.+?)\.log.gz" % logprefix

    log_name = log["name"]
    matches = re.match(backup_pattern, log_name)
    if matches is None:
        raise LogRotateError(
            "Rotated log '%s' does not match pattern '%s'." %
            (log_name, backup_pattern))

    size = int(log["size"])
    if size > 30:
        raise LogRotateError(
            "Backup log '%s' should be less than 30MB (as gzipped), "
            "but is %sMB." % (log_name, size))

    dt = matches.groups()[0]
    dt_pattern = "%Y-%m-%dT%H-%M-%S.%f"

    try:
        # note - we have to use datetime's strptime because time's doesn't
        # support partial seconds.
        dt = datetime.strptime(dt, dt_pattern)
    except Exception:
        raise LogRotateError(
            "Log for %s has invalid datetime appended: %s" % (log_name, dt))


def check_log0(expected, action_output):
    """Check that log0 exists and is not over 299MB"""
    log = action_output["results"]["result-map"].get("log0")
    if log is None:
        raise LogRotateError("No log returned from size action.")

    name = log["name"]
    if name != expected:
        raise LogRotateError(
            "Wrong unit name: Expected: %s, actual: %s" % (expected, name))

    size = int(log["size"])
    if size > 299:
        raise LogRotateError(
            "Log0 too big. Expected < 300MB, got: %sMB" % size)


def parse_args(argv=None):
    """Parse all arguments."""
    parser = add_basic_testing_arguments(
        ArgumentParser(description='Test log rotation.'), deadline=False)
    parser.add_argument(
        'agent',
        help='Which agent log rotation to test.',
        choices=['machine', 'unit'])
    return parser.parse_args(argv)


def make_client_from_args(args):
    client = client_from_config(args.env, args.juju_bin, args.debug)
    if args.temp_env_name is not None:
        client.env.set_model_name(args.temp_env_name)
    update_env(
        client.env, args.temp_env_name, series=args.series,
        bootstrap_host=args.bootstrap_host, agent_url=args.agent_url,
        agent_stream=args.agent_stream, region=args.region)
    jes_enabled = client.is_jes_enabled()
    if jes_enabled:
        client.env.juju_home = jes_home_path(client.env.juju_home,
                                             args.temp_env_name)
    client.kill_controller()
    return client


def main():
    args = parse_args()
    client = make_client_from_args(args)
    with boot_context(args.temp_env_name, client,
                      bootstrap_host=args.bootstrap_host,
                      machines=args.machine, series=args.series,
                      agent_url=args.agent_url, agent_stream=args.agent_stream,
                      log_dir=args.logs, keep_env=args.keep_env,
                      upload_tools=args.upload_tools,
                      region=args.region):
        charm_path = local_charm_path(
            charm='fill-logs', juju_ver=client.version, series='trusty')
        client.deploy(charm_path)
        if args.agent == "unit":
            test_unit_rotation(client)
        if args.agent == "machine":
            assess_machine_rotation(client)


if __name__ == '__main__':
    main()
