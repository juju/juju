"""Tests for remote access to juju machines."""

from mock import patch
import os
import subprocess
import sys

import winrm

from jujupy import (
    EnvJujuClient,
    get_timeout_path,
    JujuData,
    Status,
)
from remote import (
    remote_from_address,
    remote_from_unit,
    WinRmRemote,
)
import tests
from utility import (
    temp_dir,
)


class TestRemote(tests.FakeHomeTestCase):

    precise_status_output = """\
    machines:
        "1":
            series: precise
    applications:
        a-application:
            units:
                a-application/0:
                    machine: "1"
                    public-address: 10.55.60.1
    """

    win2012hvr2_status_output = """\
    machines:
        "2":
            series: win2012hvr2
    applications:
        a-application:
            units:
                a-application/0:
                    machine: "2"
                    public-address: 10.55.60.2
    """

    def test_remote_from_unit(self):
        env = JujuData("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-application/0"
        with patch.object(client, "get_status", autospec=True) as st:
            st.return_value = Status.from_text(self.precise_status_output)
            remote = remote_from_unit(client, unit)
        self.assertEqual(
            repr(remote),
            "<SSHRemote env='an-env' unit='a-application/0'>")
        self.assertIs(False, remote.is_windows())

    def test_remote_from_unit_with_series(self):
        env = JujuData("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-application/0"
        remote = remote_from_unit(client, unit, series="trusty")
        self.assertEqual(
            repr(remote),
            "<SSHRemote env='an-env' unit='a-application/0'>")
        self.assertIs(False, remote.is_windows())

    def test_remote_from_unit_with_status(self):
        env = JujuData("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-application/0"
        status = Status.from_text(self.win2012hvr2_status_output)
        remote = remote_from_unit(client, unit, status=status)
        self.assertEqual(
            repr(remote),
            "<WinRmRemote env='an-env' unit='a-application/0'"
            " addr='10.55.60.2'>")
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
        env = JujuData("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-application/0"
        remote = remote_from_unit(client, unit, series="trusty")
        with patch.object(client, "get_juju_output") as mock_cmd:
            mock_cmd.return_value = "contents of /a/file"
            output = remote.run("cat /a/file")
            self.assertEqual(output, "contents of /a/file")
        mock_cmd.assert_called_once_with("ssh", unit, "cat /a/file",
                                         timeout=120)

    def test_run_with_unit_fallback(self):
        env = JujuData("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-application/0"
        with patch.object(client, "get_status") as st:
            st.return_value = Status.from_text(self.precise_status_output)
            remote = remote_from_unit(client, unit)
            with patch.object(client, "get_juju_output") as mock_gjo:
                mock_gjo.side_effect = subprocess.CalledProcessError(1, "ssh",
                                                                     output="")
                with patch.object(remote, "_run_subprocess") as mock_run:
                    mock_run.return_value = "contents of /a/file"
                    output = remote.run("cat /a/file")
                    self.assertEqual(output, "contents of /a/file")
        mock_gjo.assert_called_once_with("ssh", unit, "cat /a/file",
                                         timeout=120)
        mock_run.assert_called_once_with([
            "ssh",
            "-o", "User ubuntu",
            "-o", "UserKnownHostsFile /dev/null",
            "-o", "StrictHostKeyChecking no",
            "-o", "PasswordAuthentication no",
            "10.55.60.1",
            "cat /a/file",
        ])
        self.assertRegexpMatches(
            self.log_stream.getvalue(),
            "(?m)^WARNING juju ssh to 'a-application/0' failed, .*")

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
            "-o", "PasswordAuthentication no",
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
            "-o", "PasswordAuthentication no",
            "10.55.60.1",
            "cat /a/file",
        ])

    def test_cat_on_windows(self):
        env = JujuData("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-application/0"
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
            "-rC",
            "-o", "User ubuntu",
            "-o", "UserKnownHostsFile /dev/null",
            "-o", "StrictHostKeyChecking no",
            "-o", "PasswordAuthentication no",
            "10.55.60.1:/var/log/*",
            "10.55.60.1:~/.config",
            "/local/path",
        ])

    def test_copy_on_windows(self):
        env = JujuData("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-application/0"
        dest = "/local/path"
        with patch.object(client, "get_status", autospec=True) as st:
            st.return_value = Status.from_text(self.win2012hvr2_status_output)
            response = winrm.Response(("fake output", "",  0))
            remote = remote_from_unit(client, unit)
            with patch.object(remote.session, "run_ps", autospec=True,
                              return_value=response) as mock_run:
                with patch.object(remote, "_encoded_copy_to_dir",
                                  autospec=True) as mock_cpdir:
                    remote.copy(dest, ["C:\\logs\\*", "%APPDATA%\\*.log"])
        mock_cpdir.assert_called_once_with(dest, "fake output")
        st.assert_called_once_with()
        self.assertEquals(mock_run.call_count, 1)
        self.assertRegexpMatches(
            mock_run.call_args[0][0],
            r'.*"C:\\logs\\[*]","%APPDATA%\\[*].log".*')

    def test_copy_ipv6(self):
        remote = remote_from_address("2001:db8::34")
        self.assertEqual(remote.address, "2001:db8::34")
        dest = "/local/path"
        with patch.object(remote, "_run_subprocess") as mock_run:
            remote.copy(dest, ["/var/log/*", "~/.config"])
        mock_run.assert_called_once_with([
            "scp",
            "-rC",
            "-o", "User ubuntu",
            "-o", "UserKnownHostsFile /dev/null",
            "-o", "StrictHostKeyChecking no",
            "-o", "PasswordAuthentication no",
            "[2001:db8::34]:/var/log/*",
            "[2001:db8::34]:~/.config",
            "/local/path",
        ])

    def test_run_cmd(self):
        env = JujuData("an-env", {"type": "nonlocal"})
        client = EnvJujuClient(env, None, None)
        unit = "a-application/0"
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

    def test_run_subprocess_timeout(self):
        remote = remote_from_address("10.55.60.1")
        remote.timeout = 63
        with patch("subprocess.check_output", autospec=True) as mock_co:
            remote.cat("/a/file")
        mock_co.assert_called_once_with((
            sys.executable,
            get_timeout_path(),
            "63.00",
            "--",
            "ssh",
            "-o", "User ubuntu",
            "-o", "UserKnownHostsFile /dev/null",
            "-o", "StrictHostKeyChecking no",
            "-o", "PasswordAuthentication no",
            "10.55.60.1",
            "cat /a/file",
            ),
            stdin=subprocess.PIPE,
        )

    def test_encoded_copy_to_dir_one(self):
        output = "testfile|K0ktLuECAA==\r\n"
        with temp_dir() as dest:
            WinRmRemote._encoded_copy_to_dir(dest, output)
            with open(os.path.join(dest, "testfile")) as f:
                self.assertEqual(f.read(), "test\n")

    def test_encoded_copy_to_dir_many(self):
        output = "test one|K0ktLuECAA==\r\ntest two|K0ktLuECAA==\r\n\r\n"
        with temp_dir() as dest:
            WinRmRemote._encoded_copy_to_dir(dest, output)
            for name in ("test one", "test two"):
                with open(os.path.join(dest, name)) as f:
                    self.assertEqual(f.read(), "test\n")

    def test_encoded_copy_traversal_guard(self):
        output = "../../../etc/passwd|K0ktLuECAA==\r\n"
        with temp_dir() as dest:
            with self.assertRaises(ValueError):
                WinRmRemote._encoded_copy_to_dir(dest, output)
