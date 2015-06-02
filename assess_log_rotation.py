#!/usr/bin/env python
from __future__ import print_function

__metaclass__ = type

from argparse import ArgumentParser
from datetime import datetime
import logging
import re

from deploy_stack import (
    dump_env_logs,
    get_machine_dns_name,
)
from jujuconfig import (
    get_juju_home,
)
from jujupy import (
    make_client,
    parse_new_state_server_from_error,
    temp_bootstrap_env,
    yaml_loads,
)
from utility import (
    print_now,
)


class LogRotateError(Exception):

    ''' LogRotate test Exception base class. '''

    def __init__(self, message):
        super(LogRotateError, self).__init__(message)


def test_unit_rotation(client):
    """Tests unit log rotation."""
    test_rotation(client,
                  "/var/log/juju/unit-fill-logs-0.log",
                  "unit-fill-logs-0",
                  "fill-unit",
                  "unit-size",
                  "megs=300")


def test_machine_rotation(client):
    """Tests machine log rotation."""
    test_rotation(client,
                  "/var/log/juju/machine-1.log",
                  "machine-1",
                  "fill-machine",
                  "machine-size", "megs=300", "machine=1")


def test_rotation(client, logfile, prefix, fill_action, size_action, *args):
    """A reusable help for testing log rotation.log

    Deploys the fill-logs charm and uses it to fill the machine or unit log and
    test that the logs roll over correctly.
    """

    # the rotation point should be 300 megs, so let's make sure we hit that.hit
    # we'll obviously already have some data in the logs, so adding exactly
    # 300megs should do the trick.

    # we run do_fetch here so that we wait for fill-logs to finish.
    client.action_do_fetch("fill-logs/0", fill_action, "3m", *args)
    out = client.action_do_fetch("fill-logs/0", size_action)
    action_output = yaml_loads(out)

    # Now we should have one primary log file, and one backup log file.
    # The backup should be approximately 300 megs.
    # The primary should be below 300.

    check_log0(logfile, action_output)
    check_expected_backup("log1", prefix, action_output)

    # we should only have one backup, not two.
    check_for_extra_backup("log2", action_output)

    # do it all again, this should generate a second backup.

    client.action_do_fetch("fill-logs/0", fill_action, "3m", *args)
    out = client.action_do_fetch("fill-logs/0", size_action)
    action_output = yaml_loads(out)

    # we should have two backups.
    check_log0(logfile, action_output)
    check_expected_backup("log1", prefix, action_output)
    check_expected_backup("log2", prefix, action_output)

    check_for_extra_backup("log3", action_output)

    # one more time... we should still only have 2 backups and primary

    client.action_do_fetch("fill-logs/0", fill_action, "3m", *args)
    out = client.action_do_fetch("fill-logs/0", size_action)
    action_output = yaml_loads(out)

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

    backup_pattern = "/var/log/juju/%s-(.+?)\.log" % logprefix

    log_name = log["name"]
    matches = re.match(backup_pattern, log_name)
    if matches is None:
        raise LogRotateError(
            "Rotated log '%s' does not match pattern '%s'." %
            (log_name, backup_pattern))

    size = int(log["size"])
    if size < 299 or size > 301:
        raise LogRotateError(
            "Backup log '%s' should be ~300MB, but is %sMB." %
            (log_name, size))

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
    parser = ArgumentParser('Test log rotation.')
    parser.add_argument(
        '--debug', action='store_true', default=False,
        help='Use --debug juju logging.')
    parser.add_argument(
        'agent',
        help='Which agent log rotation to test.',
        choices=['machine', 'unit'])
    parser.add_argument(
        'juju_path', help='Directory your juju binary lives in.')
    parser.add_argument(
        'env_name', help='Juju environment name to run tests in.')
    parser.add_argument('logs', help='Directory to store logs in.')
    parser.add_argument(
        'temp_env_name', nargs='?',
        help='Temporary environment name to use for this test.')
    return parser.parse_args(argv)


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
        client.wait_for_started()
        client.juju("deploy", ('local:trusty/fill-logs',))

        if args.agent == "unit":
            test_unit_rotation(client)
        if args.agent == "machine":
            test_machine_rotation(client)
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


if __name__ == '__main__':
    main()
