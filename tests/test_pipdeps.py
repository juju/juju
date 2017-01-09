"""Tests for pipdeps script."""

import mock
import os
import subprocess
import unittest


import pipdeps
import tests
import utility


class TestGetParser(unittest.TestCase):

    def test_list_defaults(self):
        parser = pipdeps.get_parser("pipdeps.py")
        args = parser.parse_args(["list"])
        self.assertEqual("list", args.command)
        self.assertEqual(os.path.expanduser("~/cloud-city"), args.cloud_city)
        self.assertEqual(False, args.verbose)

    def test_install_verbose(self):
        parser = pipdeps.get_parser("pipdeps.py")
        args = parser.parse_args(["-v", "install"])
        self.assertEqual("install", args.command)
        self.assertEqual(pipdeps.get_requirements(), args.requirements)
        self.assertEqual(True, args.verbose)

    def test_update_cloud_city(self):
        parser = pipdeps.get_parser("pipdeps.py")
        args = parser.parse_args(["--cloud-city", "/tmp/cc", "update"])
        self.assertEqual("update", args.command)
        self.assertEqual("/tmp/cc", args.cloud_city)

    def test_delete(self):
        parser = pipdeps.get_parser("pipdeps.py")
        args = parser.parse_args(["delete"])
        self.assertEqual("delete", args.command)


class TestS3Connection(unittest.TestCase):

    def test_anon(self):
        sentinel = object()
        with mock.patch("boto.s3.connection.S3Connection", autospec=True,
                        return_value=sentinel) as s3_mock:
            s3 = pipdeps.s3_anon()
            self.assertIs(s3, sentinel)
        s3_mock.assert_called_once_with(anon=True)

    def test_auth_with_rc(self):
        with utility.temp_dir() as fake_city:
            with open(os.path.join(fake_city, "ec2rc"), "wb") as f:
                f.write(
                    "AWS_SECRET_KEY=secret\n"
                    "AWS_ACCESS_KEY=access\n"
                    "export AWS_SECRET_KEY AWS_ACCESS_KEY\n"
                )
            sentinel = object()
            with mock.patch("boto.s3.connection.S3Connection", autospec=True,
                            return_value=sentinel) as s3_mock:
                s3 = pipdeps.s3_auth_with_rc(fake_city)
                self.assertIs(s3, sentinel)
        s3_mock.assert_called_once_with("access", "secret")


class TestRunPipInstall(unittest.TestCase):

    req_path = os.path.join(
        os.path.realpath(os.path.dirname(pipdeps.__file__)),
        "requirements.txt")

    def test_added_args(self):
        with mock.patch("subprocess.check_call", autospec=True) as cc_mock:
            pipdeps.run_pip_install(["--user"], self.req_path)
        cc_mock.assert_called_once_with([
            "pip", "-q", "install", "-r", self.req_path, "--user"])

    def test_verbose(self):
        with mock.patch("subprocess.check_call", autospec=True) as cc_mock:
            pipdeps.run_pip_install(
                ["--download", "/tmp/pip"], self.req_path, verbose=True)
        cc_mock.assert_called_once_with([
            "pip", "install", "-r", self.req_path, "--download", "/tmp/pip"])

    def test_pip3_install(self):
        with mock.patch("subprocess.check_call", autospec=True) as cc_mock:
            pipdeps.run_pip3_install(["--user"], self.req_path)
        cc_mock.assert_called_once_with([
            "pip3", "-q", "install", "-r", self.req_path, "--user"])


class TestRunPipUninstall(unittest.TestCase):

    def test_run_pip_uninstall(self):
        with utility.temp_dir() as base:
            obsolete = os.path.join(base, 'obsolete.txt')
            with open(obsolete, 'w') as o_file:
                o_file.write('foo (9.7.6)\nazure (0.8.0)')
            list_output = 'azure (0.8.0)\nbar (1.2.3)'
            with mock.patch("subprocess.check_output", autospec=True,
                            return_value=list_output) as co_mock:
                with mock.patch("subprocess.check_call",
                                autospec=True) as cc_mock:
                    pipdeps.run_pip_uninstall(obsolete)
        co_mock.assert_called_once_with(['pip', 'list'])
        cc_mock.assert_called_once_with(
            ['pip', 'uninstall', '-y', 'azure'])


class TestGetRequirements(unittest.TestCase):

    def test_get_requirements(self):
        dists = [
            ('Ubuntu', '16.04', 'xenial'),
            ('debian', 'squeeze/sid', ''),
            ('centos', '7.2.1511', 'Core'),
            ('', '', ''),  # Windows and MacOS
            ]
        with mock.patch('platform.dist', autospec=True,
                        side_effect=dists):
            self.assertEqual(pipdeps.REQUIREMENTS, pipdeps.get_requirements())
            self.assertEqual(pipdeps.REQUIREMENTS, pipdeps.get_requirements())
            self.assertEqual(pipdeps.MAC_WIN_REQS, pipdeps.get_requirements())

            self.assertEqual(pipdeps.MAC_WIN_REQS, pipdeps.get_requirements())


class TestIsPy3Supported(tests.TestCase):

    def test_is_py3_supported(self):
        with mock.patch("subprocess.check_output", autospec=True,
                        return_value='3.5.0') as co_mock:
            with mock.patch("subprocess.check_call", autospec=True) as cc_mock:
                self.assertTrue(pipdeps.is_py3_supported())
        co_mock.assert_called_once_with(
            ['python3', '--version'], stderr=subprocess.STDOUT)
        cc_mock.assert_called_once_with(['pip3', '--version'])

    def test_is_py3_supported_older_python3_version(self):
        with mock.patch("subprocess.check_output", autospec=True,
                        return_value='3.4') as co_mock:
            with mock.patch("subprocess.check_call", autospec=True) as cc_mock:
                self.assertFalse(pipdeps.is_py3_supported())
        co_mock.assert_called_once_with(
            ['python3', '--version'], stderr=subprocess.STDOUT)
        self.assertEqual(cc_mock.call_count, 0)

    def test_is_pyt3_supported_python3_not_installed(self):
        with mock.patch("subprocess.check_output", autospec=True,
                        side_effect=OSError(2, 'No such file')) as co_mock:
            with mock.patch("subprocess.check_call", autospec=True) as cc_mock:
                self.assertFalse(pipdeps.is_py3_supported())
        co_mock.assert_called_once_with(
            ['python3', '--version'], stderr=subprocess.STDOUT)
        self.assertEqual(cc_mock.call_count, 0)

    def test_is_pyt3_supported_pip3_not_installed(self):
        with mock.patch("subprocess.check_output", autospec=True,
                        return_value='3.5.2') as co_mock:
            with mock.patch("subprocess.check_call", autospec=True,
                            side_effect=OSError(2, 'No such file')) as cc_mock:
                self.assertFalse(pipdeps.is_py3_supported())
        co_mock.assert_called_once_with(
            ['python3', '--version'], stderr=subprocess.STDOUT)
        cc_mock.assert_called_once_with(['pip3', '--version'])
