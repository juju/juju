"""Tests for assess_persistent_storage module."""

import StringIO
from subprocess import CalledProcessError
from textwrap import dedent

from mock import (
    Mock,
    patch,
    )

import assess_persistent_storage as aps
from tests import (
    parse_error,
    TestCase,
    )
from utility import JujuAssertionError


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = aps.parse_args(
            ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                aps.parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertNotIn("TODO", fake_stdout.getvalue())


class TestGetStorageSystems(TestCase):
    def test_returns_single_known_filesystem(self):
        storage_json = dedent("""\
        filesystems:
            0/0:
                provider-id: 0/1
                storage: single-fs/1
                attachments:
                machines:
                    "0":
                    mount-point: /srv/single-fs
                    read-only: false
                    life: alive
                units:
                    dummy-storage/0:
                    machine: "0"
                    location: /srv/single-fs
                    life: alive
                pool: rootfs
                size: 28775
                life: alive
                status:
                current: attached
                since: 14 Mar 2017 17:01:15+13:00
        """)
        client = Mock()
        client.list_storage.return_value = storage_json

        self.assertEqual(
            aps.get_storage_filesystems(client, 'single-fs'),
            ['single-fs/1'])

    def test_returns_empty_list_when_none_found(self):
        storage_json = dedent("""\
        filesystems:
            0/0:
                provider-id: 0/1
                storage: single-fs/1
                attachments:
                machines:
                    "0":
                    mount-point: /srv/single-fs
                    read-only: false
                    life: alive
                units:
                    dummy-storage/0:
                    machine: "0"
                    location: /srv/single-fs
                    life: alive
                pool: rootfs
                size: 28775
                life: alive
                status:
                current: attached
                since: 14 Mar 2017 17:01:15+13:00
        """)
        client = Mock()
        client.list_storage.return_value = storage_json

        self.assertEqual(
            aps.get_storage_filesystems(client, 'not-found'),
            [])

    def test_returns_many_for_multiple_finds(self):
        storage_json = dedent("""\
        filesystems:
            "0/0":
                provider-id: 0/1
                storage: multi-fs/1
                attachments:
                machines:
                    "0":
                    mount-point: /srv/multi-fs
                    read-only: false
                    life: alive
                units:
                    dummy-storage/0:
                    machine: "0"
                    location: /srv/multi-fs
                    life: alive
                pool: rootfs
                size: 28775
                life: alive
                status:
                current: attached
                since: 14 Mar 2017 17:01:15+13:00
            0/1:
                provider-id: 0/2
                storage: multi-fs/2
                attachments:
                machines:
                    "0":
                    mount-point: /srv/multi-fs
                    read-only: false
                    life: alive
                units:
                    dummy-storage/0:
                    machine: "0"
                    location: /srv/multi-fs
                    life: alive
                pool: rootfs
                size: 28775
                life: alive
                status:
                current: attached
                since: 14 Mar 2017 17:01:15+13:00
        """)
        client = Mock()
        client.list_storage.return_value = storage_json

        self.assertEqual(
            aps.get_storage_filesystems(client, 'multi-fs'),
            ['multi-fs/1', 'multi-fs/2'])


class TestAssertStorageIsIntact(TestCase):

    def test_passes_when_token_values_match(self):
        client = Mock()
        stored_values = {'single-fs-token': 'abc123'}
        expected_results = {'single-fs-token': 'abc123'}
        with patch.object(
                aps, 'get_stored_token_content',
                autospec=True,
                return_value=stored_values) as m_gstc:
            aps.assert_storage_is_intact(client, expected_results)
            m_gstc.assert_called_once_with(client)

    def test_ignores_token_values_not_supplied(self):
        client = Mock()
        stored_values = {
            'single-fs-token': 'abc123',
            'multi-fs-token/1': '00000'
        }
        expected_results = {'single-fs-token': 'abc123'}
        with patch.object(
                aps, 'get_stored_token_content',
                autospec=True,
                return_value=stored_values) as m_gstc:
            aps.assert_storage_is_intact(client, expected_results)
            m_gstc.assert_called_once_with(client)

    def test_raises_when_token_values_do_not_match(self):
        client = Mock()
        stored_values = {'single-fs-token': 'abc123x'}
        expected_results = {'single-fs-token': 'abc123'}
        with patch.object(
                aps, 'get_stored_token_content',
                autospec=True,
                return_value=stored_values):
            with self.assertRaises(JujuAssertionError):
                aps.assert_storage_is_intact(client, expected_results)


class TestGetStoredTokenContent(TestCase):

    def test_raises_if_token_file_not_present(self):
        client = Mock()
        client.get_juju_output.side_effect = CalledProcessError(-1, None, '')
        with self.assertRaises(JujuAssertionError):
            aps.get_stored_token_content(client)

    def test_returns_dict_containing_all_values_when_single_value(self):
        token_file_contents = dedent("""\

        single-fs-token:Blocked: not set
        """)
        client = Mock()
        client.get_juju_output.return_value = token_file_contents

        self.assertEqual(
            aps.get_stored_token_content(client),
            {'single-fs-token': 'Blocked: not set'})

    def test_returns_dict_containing_all_values_when_many_values(self):
        token_file_contents = dedent("""\

        single-fs-token:Blocked: not set
        multi-fs-token/2:abc123
        """)
        client = Mock()
        client.get_juju_output.return_value = token_file_contents

        self.assertEqual(
            aps.get_stored_token_content(client),
            {
                'single-fs-token': 'Blocked: not set',
                'multi-fs-token/2': 'abc123'
            })
