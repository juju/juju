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
            env.deploy('local:{}/fill-logs'.format(env.config.get(
                'default-series', 'trusty')))
            env.wait_for_started()

            test_unit_rotation(env)


        except subprocess.CalledProcessError as e:
            if getattr(e, 'stderr', None) is not None:
                sys.stderr.write(e.stderr)
            raise
    finally:
        env.destroy_environment()


def test_unit_rotation(env):
    # the rotation point should be 300 megs, so let's make sure we hit that.hit
    # we'll obviously already have some data in the logs, so adding exactly 300megs
    # should do the trick.
    env.action_do("fill-logs/0", "fill-unit", "megs=300")
    output = env.action_do_fetch("fill-logs/0", "unit-size")
    obj = yaml_loads(output)

    # Now we should have one primary log file, and one backup log file.
    # The backup should be approximately 300 megs.
    # The primary should be below 300.
    check_unit_log0(obj)
    check_unit_backup("log1", obj)

    # we should only have one backup, not two.
    log2 = obj["results"]["result-map"]["log2"]
    if log2 is not None:
        raise Exception("Extra backup unit log after rotation: " + log2["name"])

    # do it all again, this should generate a second backup.

    env.action_do("fill-logs/0", "fill-unit", "megs=300")
    output = env.action_do_fetch("fill-logs/0", "unit-size")
    print(output)
    obj = yaml_loads(output)

    check_unit_log0(obj)
    check_unit_backup("log1", obj)
    check_unit_backup("log2", obj)

    log3 = obj["results"]["result-map"]["log3"]
    if log3 is not None:
        raise Exception("Extra backup unit log after second rotation: " + log2["name"])

    # one more time... we should still only have 2 backups and primary

    env.action_do("fill-logs/0", "fill-unit", "megs=300")
    output = env.action_do_fetch("fill-logs/0", "unit-size")
    obj = yaml_loads(output)

    check_unit_log0(obj)
    check_unit_backup("log1", obj)
    check_unit_backup("log2", obj)

    log3 = obj["results"]["result-map"]["log3"]
    if log3 is not None:
        raise Exception("Extra backup unit log after second rotation: " + log2["name"])


def check_unit_backup(logname, yaml_obj):
    log = yaml_obj["results"]["result-map"][logname]
    if log is None:
        raise Exception(format("Missing backup unit log '{}'' after rotation.", logname))

    backup_pattern_string = "/var/log/juju/unit-fill-logs-0(.+?)\.log"
    backup_pattern = re.compile(backup_pattern_string)

    log_name = log["name"]
    matches = re.match(backup_pattern, log_name)
    if len(matches) < 2:
        raise Exception(format("Rotated unit log name '{}' does not match pattern '{}'.", log_name, backup_pattern_string))

    size = int(log["size"])
    if size < 300 or size > 301:
        raise Exception(format("Unit log name '{}' should be close to 300MB, but is {}MB.", size))

    dt = matches[1]
    dt_pattern = "%Y-%m-%dT%H-%M-%S.%f"

    try:
        # note - we have to use datetime's strptime because time's doesn't
        # support partial seconds.
        dt = datetime.strptime(dt, dt_pattern)
    except Exception:
        raise Exception(format("Rotated unit log name for {} has invalid datetime appended: {}", logname, dt))


def check_unit_log0(obj):
    log0 = obj["results"]["result-map"]["log0"]
    if log0 is None:
        raise Exception("No unit log returned from unit-size action.")

    expected = "/var/log/juju/unit-fill-logs-0.log"
    name = log0["name"]
    if name != expected:
        raise Exception(format("Wrong unit name from action result. Expected: {}, actual: {}", expected, name))

    size = int(log0["size"])
    if size > 300:
        raise Exception(format("Unit log not rolled. Expected size < 300MB, got: {}MB", size))


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
