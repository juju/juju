#!/usr/bin/env python3
"""Validate ability of the user to import and remove ssh keys"""

from __future__ import print_function

import argparse
import logging
import re
import subprocess
import sys

from deploy_stack import (
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


__metaclass__ = type


log = logging.getLogger("assess_ssh_keys")


class SSHKey:

    def __init__(self, fingerprint, comment):
        self.fingerprint = fingerprint
        self.comment = comment

    @classmethod
    def from_fingerprint_line(cls, line):
        fingerprint, comment = line.split(" ", 1)
        if False:
            raise ValueError("Not an ssh fingerprint: {!r}".format(line))
        if comment.startswith("(") and comment.endswith(")"):
            comment = comment[1:-1]
        return cls(fingerprint, comment)

    def __str__(self):
        return "{} ({})".format(self.fingerprint, self.comment)

    def __repr__(self):
        return "{}({}, {})".format(
            self.__class__.__name__, self.fingerprint, self.comment)


_KEYS_LEAD_MODEL = "Keys used in model: "
_KEYS_LEAD_ADMIN = "Keys for user admin:"


def parse_ssh_keys_output(output, expected_model):
    """Parse and validate output from `juju ssh-keys` command."""
    if not output.startswith((_KEYS_LEAD_MODEL, _KEYS_LEAD_ADMIN)):
        raise AssertionError("Invalid ssh-keys output: {!r}".format(output))
    lines = output.splitlines()
    model = lines[0].split(_KEYS_LEAD_MODEL, 1)[-1]
    if model != _KEYS_LEAD_ADMIN and expected_model != model:
        raise AssertionError("Expected keys for model: {} got: {}".format(
            expected_model, model))
    return [SSHKey.from_fingerprint_line(line) for line in lines[1:]]


def expect_juju_failure(fail_pattern, method, *args, **kwargs):
    """Assert method fails with expected output included."""
    fail_re = re.compile(fail_pattern, re.MULTILINE)
    try:
        output = method(*args, **kwargs)
    except subprocess.CalledProcessError as e:
        # The errors go to stderr, but as the current behaviour is to not
        # exit calls will have merged stderr into stdout, so check output.
        if fail_re.search(e.output) is None:
            raise AssertionError(
                "Juju failed with output not matching: {!r} {!r}".format(
                    e.output, fail_pattern))
    else:
        if fail_re.search(output) is None:
            raise AssertionError(
                "Juju did not fail with output matching: {!r} {!r}".format(
                    output, fail_pattern))
        log.info("Error found in output but the juju process exited 0.")


VALID_KEY = (
    "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7ibpRhiMie+Ytu5XqSPrvuXol1LMVztWWS"
    "Tuja0As95VvqoCBxyKMmtnROYGhwF2BUHdHD5HdrwJ5WpIxhh+APhBuI9fZ52YbFhcxU/NxQ1"
    "y8xw2sfm8HH0DGeg3ssRWzFVUTJ4QOAkJzy2zxiK3BfwQr5W5UIDnAtMBv56J7E4DFe6skabn"
    "dWxOP8JzLtNFr/w3p/yAh/Akv6eJus8fBCKNYYy1/A+sUAZc/+dZLxk5qtfXqwIMtxFtK39vf"
    "BlvVU0tpMAPhaEb/Vzq7Zyj3nscPGjNXE2g7TUvhlKCA5tdjWbug9U2YqwowwYfz/RE3qvXfZ"
    "GtNpuBvxaXWDgpp example-key"
)


def assert_has_full_key(client, key):
    output = client.ssh_keys(full=True)
    if key not in output.splitlines()[1:]:
        raise AssertionError(
            "Expected key not found:\nwant: {}\ngot: {}".format(key, output))
    log.info("Found full key as expected")


def assert_has_key_matching_comment(client, comment_pattern):
    log.info("Expecting key with comment matching: %r", comment_pattern)
    comment_re = re.compile(comment_pattern)
    found = False
    keys = parse_ssh_keys_output(client.ssh_keys(), client.env.environment)
    for key in keys:
        if comment_re.match(key.comment) is not None:
            found = True
            log.info("Matching key found: %s", key)
            # No break so all matches are logged
    if not found:
        raise AssertionError(
            "No keys matching comment:\npattern: {!r}\nkeys: {}".format(
                comment_pattern, "\n".join(map(str, keys))))


def _assess_remove_internal_key(client, name):
    pattern = r'^cannot remove key id "{0}": may not delete internal key: {0}$'
    expect_juju_failure(pattern.format(name), client.remove_ssh_key, name)


def assess_ssh_keys(client):
    initial_keys_output = client.ssh_keys()
    initial_keys = parse_ssh_keys_output(
        initial_keys_output, client.env.environment)
    log.info(
        "Initial keys in default model:\n%s",
        "\n".join(map(str, initial_keys)))

    log.info("Testing expected error when adding an invalid key")
    pattern = r'cannot add key "badness": invalid ssh key: badness$'
    expect_juju_failure(pattern, client.add_ssh_key, "badness")

    log.info("Testing success when adding a valid key")
    client.add_ssh_key(VALID_KEY)
    assert_has_full_key(client, VALID_KEY)

    log.info("Testing expected error when adding duplicate key")
    pattern = r'^cannot add key ".*": duplicate ssh key: .*$'
    expect_juju_failure(pattern, client.add_ssh_key, VALID_KEY)

    log.info("Testing success when importing keys from github")
    client.import_ssh_key("gh:sinzui")
    assert_has_key_matching_comment(client, r'.*gh:sinzui')

    log.info("Testing success when importing keys from launchpad")
    client.import_ssh_key("lp:gz")
    assert_has_key_matching_comment(client, r'.*lp:gz')

    log.info("Testing expected error when removing a non-existent key")
    pattern = r'^cannot {0} key id "{1}": invalid ssh key: {1}$'.format(
        "delete" if client.is_juju1x() else "remove", "no-such-key")
    expect_juju_failure(pattern, client.remove_ssh_key, "no-such-key")

    log.info("Testing expected error removing the juju internal keys")
    if client.is_juju1x():
        log.info("...skipped on juju version %s", client.version)
    else:
        _assess_remove_internal_key(client, "juju-client-key")
        _assess_remove_internal_key(client, "juju-system-key")

    log.info("TODO test behavior when multiple models are involved")
    log.info("TODO test removing keys by both comment and fingerprint")


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test juju ssh key handling")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_ssh_keys(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
