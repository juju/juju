"""Tests for remote access to juju machines."""

import logging
from mock import patch
from StringIO import StringIO
import subprocess
import unittest

import winrm

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    Status,
)
from remote import (
    remote_from_address,
    remote_from_unit,
)


class TestRemote(unittest.TestCase):

    precise_status_output = """\
    machines:
        "1":
            series: precise
    services:
        a-service:
            units:
                a-service/0:
                    machine: "1"
                    public-address: 10.55.60.1
    """

    win2012hvr2_status_output = """\
    machines:
        "2":
            series: win2012hvr2
    services:
        a-service:
            units:
                a-service/0:
                    machine: "2"
                    public-address: 10.55.60.2
    """

    def setUp(self):
        log = logging.getLogger()
        self.addCleanup(setattr, log, "handlers", log.handlers)
        log.handlers = []
        self.log_stream = StringIO()
        handler = logging.StreamHandler(self.log_stream)
        handler.setFormatter(logging.Formatter("%(levelname)s %(message)s"))
        log.addHandler(handler)

    def test_remote_from_unit(self):
        env = SimpleEnvironment("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-service/0"
        with patch.object(client, "get_status", autospec=True) as st:
            st.return_value = Status.from_text(self.precise_status_output)
            remote = remote_from_unit(client, unit)
        self.assertEqual(
            repr(remote),
            "<SSHRemote env='an-env' unit='a-service/0'>")
        self.assertIs(False, remote.is_windows())

    def test_remote_from_unit_with_series(self):
        env = SimpleEnvironment("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-service/0"
        remote = remote_from_unit(client, unit, series="trusty")
        self.assertEqual(
            repr(remote),
            "<SSHRemote env='an-env' unit='a-service/0'>")
        self.assertIs(False, remote.is_windows())

    def test_remote_from_unit_with_status(self):
        env = SimpleEnvironment("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-service/0"
        status = Status.from_text(self.win2012hvr2_status_output)
        remote = remote_from_unit(client, unit, status=status)
        self.assertEqual(
            repr(remote),
            "<WinRmRemote env='an-env' unit='a-service/0' addr='10.55.60.2'>")
        self.assertIs(True, remote.is_windows())

    def test_remote_from_address(self):
        remote = remote_from_address("10.55.60.1")
        self.assertEqual(repr(remote), "<SSHRemote addr='10.55.60.1'>")
        self.assertIs(None, remote.is_windows())

    def test_remote_from_address_and_series(self):
        remote = remote_from_address("10.55.60.2", series="trusty")
        self.assertEqual(repr(remote), "<SSHRemote addr='10.55.60.2'>")
        self.assertIs(False, remote.is_windows())

    def test_remote_from_address_and_win_series(self):
        remote = remote_from_address("10.55.60.3", series="win2012hvr2")
        self.assertEqual(repr(remote), "<WinRmRemote addr='10.55.60.3'>")
        self.assertIs(True, remote.is_windows())

    def test_run_with_unit(self):
        env = SimpleEnvironment("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-service/0"
        remote = remote_from_unit(client, unit, series="trusty")
        with patch.object(client, "get_juju_output") as mock_cmd:
            mock_cmd.return_value = "contents of /a/file"
            output = remote.run("cat /a/file")
            self.assertEqual(output, "contents of /a/file")
        mock_cmd.assert_called_once_with("ssh", unit, "cat /a/file")

    def test_run_with_unit_fallback(self):
        env = SimpleEnvironment("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-service/0"
        with patch.object(client, "get_status") as st:
            st.return_value = Status.from_text(self.precise_status_output)
            remote = remote_from_unit(client, unit)
            with patch.object(client, "get_juju_output") as mock_cmd:
                mock_cmd.side_effect = subprocess.CalledProcessError(1, "ssh")
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
            "10.55.60.1",
            "cat /a/file",
        ])
        self.assertRegexpMatches(
            self.log_stream.getvalue(),
            "(?m)^WARNING juju ssh to 'a-service/0' failed: .*")

    def test_run_with_address(self):
        remote = remote_from_address("10.55.60.1")
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

    def test_cat(self):
        remote = remote_from_address("10.55.60.1")
        with patch.object(remote, "_run_subprocess") as mock_run:
            remote.cat("/a/file")
        mock_run.assert_called_once_with([
            "ssh",
            "-o", "User ubuntu",
            "-o", "UserKnownHostsFile /dev/null",
            "-o", "StrictHostKeyChecking no",
            "10.55.60.1",
            "cat /a/file",
        ])

    def test_cat_on_windows(self):
        env = SimpleEnvironment("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-service/0"
        with patch.object(client, "get_status", autospec=True) as st:
            st.return_value = Status.from_text(self.win2012hvr2_status_output)
            response = winrm.Response(("contents of /a/file", "",  0))
            remote = remote_from_unit(client, unit)
            with patch.object(remote.session, "run_cmd", autospec=True,
                              return_value=response) as mock_run:
                output = remote.cat("/a/file")
                self.assertEqual(output, "contents of /a/file")
        st.assert_called_once_with()
        mock_run.assert_called_once_with("type", ["/a/file"])

    def test_copy(self):
        remote = remote_from_address("10.55.60.1")
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

    def test_run_cmd(self):
        env = SimpleEnvironment("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-service/0"
        with patch.object(client, "get_status", autospec=True) as st:
            st.return_value = Status.from_text(self.win2012hvr2_status_output)
            response = winrm.Response(("some out", "some err",  0))
            remote = remote_from_unit(client, unit)
            with patch.object(remote.session, "run_cmd", autospec=True,
                              return_value=response) as mock_run:
                output = remote.run_cmd(
                    ["C:\\Program Files\\bin.exe", "/IN", "Bob's Stuff"])
                self.assertEqual(output, response)
        st.assert_called_once_with()
        mock_run.assert_called_once_with(
            '"C:\\Program Files\\bin.exe"', ['/IN "Bob\'s Stuff"'])
