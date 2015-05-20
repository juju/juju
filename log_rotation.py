#!/usr/bin/env python
from __future__ import print_function

__metaclass__ = type

from argparse import ArgumentParser
import os
import re
import subprocess
import sys
from datetime import datetime

from jujupy import (
    CannotConnectEnv,
    Environment,
    yaml_loads
)


def test_log_rotation(environment, debug):
    """"Deploy a test charm in the specified environment. The charm will write
    gobs of data to the unit log file, which should then rotate.

    :param environment: The name of the desired environment.
    returns the environment
    """
    env = Environment.from_config(environment)
    env.client.debug = debug
    # Clean up any leftover junk
    env.destroy_environment()
    env.bootstrap()
    try:
        # wait for status info....
        try:
            try:
                env.get_status()
            except CannotConnectEnv:
                print("Status got Unable to connect to env.  Retrying...")
                env.get_status()
            env.wait_for_started()
            env.deploy('local:trusty/fill-logs')
            env.wait_for_started()

            test_unit_rotation(env)


        except subprocess.CalledProcessError as e:
            if getattr(e, 'stderr', None) is not None:
                sys.stderr.write(e.stderr)
            raise
    finally:
        # env.destroy_environment()
        pass


def test_unit_rotation(env):
    # the rotation point should be 300 megs, so let's make sure we hit that.hit
    # we'll obviously already have some data in the logs, so adding exactly 300megs
    # should do the trick.

    # we run do_fetch here so that we wait for fill-logs to finish.
    env.action_do_fetch("fill-logs/0", "fill-unit", "megs=300")
    output = env.action_do_fetch("fill-logs/0", "unit-size")
    obj = yaml_loads(output)

    # Now we should have one primary log file, and one backup log file.
    # The backup should be approximately 300 megs.
    # The primary should be below 300.
    check_unit_log0(obj)
    check_unit_backup("log1", obj)

    # we should only have one backup, not two.
    check_extra_backup("log2", obj)

    # do it all again, this should generate a second backup.

    env.action_do_fetch("fill-logs/0", "fill-unit", "megs=300")
    output = env.action_do_fetch("fill-logs/0", "unit-size")
    print(output)
    obj = yaml_loads(output)

    check_unit_log0(obj)
    check_unit_backup("log0", obj)
    check_unit_backup("log1", obj)

    # we should have two backups.
    check_extra_backup("log3", obj)

    # one more time... we should still only have 2 backups and primary

    env.action_do_fetch("fill-logs/0", "fill-unit", "megs=300")
    output = env.action_do_fetch("fill-logs/0", "unit-size")
    obj = yaml_loads(output)

    check_unit_log0(obj)
    check_unit_backup("log1", obj)
    check_unit_backup("log2", obj)

    # we should have two backups.
    check_extra_backup("log3", obj)


def check_extra_backup(logname, yaml_obj):
    try:
        # this should raise a KeyError
        log = yaml_obj["results"]["result-map"][logname]
        try:
            # no exception! log exists.
            name = log["name"]
            raise Exception("Extra backup unit log after rotation: " + name)
        except KeyError:
            # no name for log for ome reason
            raise Exception("Extra backup unit log (with no name) after rotation")
    except KeyError:
        # this is correct
        pass


def check_unit_backup(logname, yaml_obj):
    try:
        log = yaml_obj["results"]["result-map"][logname]
    except KeyError:
        raise Exception("Missing backup unit log '{}'' after rotation.".format(logname))

    backup_pattern_string = "/var/log/juju/unit-fill-logs-0-(.+?)\.log"
    backup_pattern = re.compile(backup_pattern_string)

    log_name = log["name"]
    matches = re.match(backup_pattern, log_name)
    if matches is None:
        raise Exception("Rotated unit log name '{}' does not match pattern '{}'.".format(log_name, backup_pattern_string))

    size = int(log["size"])
    if size < 299 or size > 301:
        raise Exception("Unit log name '{}' should be close to 300MB, but is {}MB.".format(log_name, size))

    dt = matches.groups()[0]
    dt_pattern = "%Y-%m-%dT%H-%M-%S.%f"

    try:
        # note - we have to use datetime's strptime because time's doesn't
        # support partial seconds.
        dt = datetime.strptime(dt, dt_pattern)
    except Exception:
        raise Exception("Rotated unit log name for {} has invalid datetime appended: {}".format(log_name, dt))


def check_unit_log0(obj):
    log = obj["results"]["result-map"]["log0"]
    if log is None:
        raise Exception("No unit log returned from unit-size action.")

    expected = "/var/log/juju/unit-fill-logs-0.log"
    name = log["name"]
    if name != expected:
        raise Exception("Wrong unit name from action result. Expected: {}, actual: {}".format(expected, name))

    size = int(log["size"])
    if size > 299:
        raise Exception("Unit log not rolled. Expected size < 300MB, got: {}MB".format(size))


def main():
    parser = ArgumentParser('Test log rotation')
    parser.add_argument('env', help='The juju environment to test')
    args = parser.parse_args()
    debug = bool(os.environ.get('DEBUG') == 'true')
    # try:
    test_log_rotation(args.env, debug)
    # except Exception as e:
    #    print('%s: %s' % (type(e), e))
    #    sys.exit(1)


if __name__ == '__main__':
    main()
