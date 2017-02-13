"""Tests for assess_juju_sync_tools module."""

import os

from mock import (
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
    temp_dir,
    )


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)


class TestAssess(TestCase):
    def test_get_agent_version_without_rc(self):
        juju_bin = "/path/juju"
        with patch('jujupy.ModelClient.get_version',
                   return_value='1.25-arch-series'):
            agent_version = get_agent_version(juju_bin)
            self.assertEquals(agent_version, "1.25")

    def test_get_agent_version_with_rc(self):
        juju_bin = "/path/juju"
        with patch('jujupy.ModelClient.get_version',
                   return_value='2.0-rc2-arch-series'):
            agent_version = get_agent_version(juju_bin)
            self.assertEquals(agent_version, "2.0")

    def test_get_agent_version_with_major_minor(self):
        juju_bin = "/path/juju"
        with patch('jujupy.ModelClient.get_version',
                   return_value='2.0.1-xenial-amd64'):
            agent_version = get_agent_version(juju_bin)
            self.assertEquals(agent_version, "2.0.1")

    def test_assert_file_version_matches_agent_version(self):
        with self.assertRaises(JujuAssertionError):
            assert_file_version_matches_agent_version(
                "2.0.1-xenial-amd64", "2.0.2")

    def test_verify_agent_tools(self):
        with temp_dir() as base_dir:
            agent_dir = os.path.join(base_dir, "tools", "released")
            os.makedirs(agent_dir)
            for filename in ['juju-2.0.1-centos7-amd64.tgz',
                             'juju-2.0.1-precise-amd64.tgz',
                             'juju-2.0.1-win2016-amd64.tgz']:
                agent_file = os.path.join(agent_dir, filename)
                open(agent_file, 'a').close()
            verify_agent_tools(base_dir, "released", "2.0.1")
            self.assertItemsEqual(['juju-2.0.1-centos7-amd64.tgz',
                                   'juju-2.0.1-precise-amd64.tgz',
                                   'juju-2.0.1-win2016-amd64.tgz'],
                                  os.listdir(agent_dir))
            self.assertIn("juju sync-tool verification done successfully",
                          self.log_stream.getvalue())

    def test_verify_agent_tools_fail(self):
        with temp_dir() as base_dir:
            agent_dir = os.path.join(base_dir, "tools", "released")
            os.makedirs(agent_dir)
            for filename in ['juju-2.0.1-centos7-amd64.tgz',
                             'juju-2.0.2-precise-amd64.tgz',
                             'juju-2.0.1-win2016-amd64.tgz']:
                agent_file = os.path.join(agent_dir, filename)
                open(agent_file, 'a').close()
            with self.assertRaises(JujuAssertionError):
                verify_agent_tools(base_dir, "released", "2.0.1")
