"""Tests for remote access to juju machines."""

import logging
from mock import patch
from StringIO import StringIO
import subprocess
import unittest

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    Status,
)
from remote import (
    Remote,
)


class TestRemote(unittest.TestCase):

    def setUp(self):
        log = logging.getLogger()
        self.addCleanup(setattr, log, "handlers", log.handlers)
        log.handlers = []
        self.log_stream = StringIO()
        handler = logging.StreamHandler(self.log_stream)
        handler.setFormatter(logging.Formatter("%(levelname)s %(message)s"))
        log.addHandler(handler)

    def test_create_with_client(self):
        env = SimpleEnvironment("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-service/0"
        remote = Remote(client, unit)
        self.assertEqual(
            repr(remote),
            "<Remote env='an-env' unit='a-service/0'>")

    def test_create_with_address(self):
        remote = Remote(address="10.55.60.1")
        self.assertEqual(repr(remote), "<Remote addr='10.55.60.1'>")

    def test_create_required_args(self):
        env = SimpleEnvironment("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        self.assertRaises(ValueError, Remote, client=client)
        self.assertRaises(ValueError, Remote, unit="a-unit/0")

    def test_run_with_unit(self):
        env = SimpleEnvironment("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-service/0"
        remote = Remote(client, unit)
        with patch.object(client, "get_juju_output") as mock_cmd:
            mock_cmd.return_value = "contents of /a/file"
            output = remote.run("cat /a/file")
            self.assertEqual(output, "contents of /a/file")
        mock_cmd.assert_called_once_with("ssh", unit, "cat /a/file")

    def test_run_with_unit_fallback(self):
        env = SimpleEnvironment("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-service/0"
        status_output = """\
        services:
            a-service:
                units:
                    a-service/0:
                        public-address: 10.55.60.2
        """
        remote = Remote(client, unit)
        with patch.object(client, "get_juju_output") as mock_cmd:
            mock_cmd.side_effect = subprocess.CalledProcessError(1, "ssh")
            with patch.object(client, "get_status") as mock_status:
                mock_status.return_value = Status.from_text(status_output)
                with patch.object(remote, "_run_subprocess") as mock_run:
                    mock_run.return_value = "contents of /a/file"
                    output = remote.run("cat /a/file")
                    self.assertEqual(output, "contents of /a/file")
        mock_cmd.assert_called_once_with("ssh", unit, "cat /a/file")
        mock_run.assert_called_once_with([
            "ssh",
            "-o", "User ubuntu",
            "-o", "UserKnownHostsFile /dev/null",
            "-o", "StrictHostKeyChecking no",
            "10.55.60.2",
            "cat /a/file",
        ])
        self.assertRegexpMatches(
            self.log_stream.getvalue(),
            "(?m)^WARNING juju ssh to 'a-service/0' failed: .*")

    def test_run_with_address(self):
        remote = Remote(address="10.55.60.1")
        with patch.object(remote, "_run_subprocess") as mock_run:
            mock_run.return_value = "contents of /a/file"
            output = remote.run("cat /a/file")
            self.assertEqual(output, "contents of /a/file")
        mock_run.assert_called_once_with([
            "ssh",
            "-o", "User ubuntu",
            "-o", "UserKnownHostsFile /dev/null",
            "-o", "StrictHostKeyChecking no",
            "10.55.60.1",
            "cat /a/file",
        ])

    def test_copy(self):
        remote = Remote(address="10.55.60.1")
        dest = "/local/path"
        with patch.object(remote, "_run_subprocess") as mock_run:
            remote.copy(dest, ["/var/log/*", "~/.config"])
        mock_run.assert_called_once_with([
            "scp",
            "-C",
            "-o", "User ubuntu",
            "-o", "UserKnownHostsFile /dev/null",
            "-o", "StrictHostKeyChecking no",
            "10.55.60.1:/var/log/*",
            "10.55.60.1:~/.config",
            "/local/path",
        ])
