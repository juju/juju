"""Tests for assess_juju_sync_tools module."""

import os

from mock import (
    call,
    patch,
    )
from assess_juju_sync_tools import (
    assert_file_version_matches_agent_version,
    verify_agent_tools,
    get_agent_version,
    parse_args,
    )
from tests import (
    TestCase,
    )
from utility import (
    JujuAssertionError,
    )
from jujupy import (
    fake_juju_client,
    )


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)


class TestAssertFileVersionMatchesAgentVersion(TestCase):
    def test_assert_file_version_matches_agent_version_valid(self):
        for version in [("2.0.1-xenial-amd64", "2.0.1"),
                        ("2.1-beta1-zesty-amd64.tgz", "2.1-beta1"),
                        ("2.0-rc2-arch-series", "2.0-rc2"),
                        ("2.0-xenial-amd64", "2.0"),
                        ("2.1-rc1", "2.1-rc1")]:
            assert_file_version_matches_agent_version(
                version[0], version[1])

    def test_raises_exception_when_versions_dont_match(self):
        for version in [("2.0.1-xenial-amd64", "2.2.1"),
                        ("2.0.2-rc2", "2.0.1"),
                        ("2.0-rc2-arch-series", "2.1")]:
            with self.assertRaises(JujuAssertionError):
                    assert_file_version_matches_agent_version(
                        version[0], version[1])


class TestAgentVersion(TestCase):
    def test_get_agent_version(self):
        for version in [("1.25-arch-series", "1.25"),
                        ("2.0-rc2-arch-series", "2.0-rc2"),
                        ("2.0.2-rc2-arch-series", "2.0.2-rc2")]:
            client = fake_juju_client(version=version[0])
            agent_version = get_agent_version(client)
            self.assertEquals(agent_version, version[1])

    def test_get_agent_version_to_fail(self):
        for version in [("2.0.2-rc2-arch-series", "2.0.2"),
                        ("2.0.2-rc2-arch-series", "2.0.2-rc1"),
                        ("2.0.2-arch-series", "2.0.2-rc1")]:
            client = fake_juju_client(version=version[0])
            agent_version = get_agent_version(client)
            self.assertNotEquals(agent_version, version[1])


class TestVerifyAgentTools(TestCase):
    def test_doesnt_raise_on_match_version(self):
        with patch.object(os, 'listdir') as lstdir:
            lstdir.return_value = [
                'juju-2.0.1-centos7-amd64.tgz',
                'juju-2.0.1-precise-amd64.tgz',
                'juju-2.0.1-win2016-amd64.tgz']
            verify_agent_tools("foo", "2.0.1")
            self.assertIn("juju sync-tool verification done successfully",
                          self.log_stream.getvalue())

    def test_ignores_none_tgz_files_on_verify_agent_tool(self):
        juju_bin_ver = "2.0.1"
        with patch(
                "assess_juju_sync_tools."
                "assert_file_version_matches_agent_version")\
                as asm:
            with patch.object(os, 'listdir') as lstdir:
                lstdir.return_value = [
                    'juju-2.0.1-centos7-amd64.tgz',
                    'juju-2.0.1-precise-amd64.tgz',
                    'juju-2.0.1-win2016-amd64.tgz',
                    'juju-2.0.1-win2016-amd64.txt']
                verify_agent_tools("foo", juju_bin_ver)
                calls = [call('juju-2.0.1-centos7-amd64.tgz',
                              "juju-{}".format(juju_bin_ver)),
                         call('juju-2.0.1-precise-amd64.tgz',
                              "juju-{}".format(juju_bin_ver)),
                         call('juju-2.0.1-win2016-amd64.tgz',
                              "juju-{}".format(juju_bin_ver))]
                self.assertEquals(asm.call_count, 3)
                for x in range(len(call)):
                    self.assertEquals(asm.call_args_list[x], calls[x])

    def test_raise_assertion_on_mismatch_version(self):
        juju_bin_ver = "2.0.1"
        with patch.object(os, 'listdir') as lstdir:
            lstdir.return_value = [
                'juju-2.0.1-centos7-amd64.tgz',
                'juju-2.0.2-precise-amd64.tgz',
                'juju-2.0.1-win2016-amd64.tgz']
            with self.assertRaises(JujuAssertionError):
                verify_agent_tools("foo", juju_bin_ver)
