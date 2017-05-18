from mock import patch
import os
import re
from StringIO import StringIO
from unittest import TestCase

from publish_streams import (
    CPCS,
    diff_files,
    get_remote_file,
    main,
    parse_args,
    publish,
    verify_metadata,
)

from utils import temp_dir


class PublishStreamsTestCase(TestCase):

    def test_parse_args(self):
        args = parse_args(
            ['-d', '-v', '-r', 'testing', 'released', '~/juju-dist', 'aws'])
        self.assertTrue(args.dry_run)
        self.assertTrue(args.verbose)
        self.assertEqual('testing', args.remote_root)
        self.assertEqual('released', args.stream)
        self.assertEqual(os.path.expanduser('~/juju-dist'), args.location)
        self.assertEqual('aws', args.cloud)

    @patch('publish_streams.publish', autospec=True)
    def test_main(self, p_mock):
        exit_code = main(['-r', 'testing', 'released', '~/juju-dist', 'aws'])
        self.assertEqual(0, exit_code)
        p_mock.assert_called_with(
            'released', os.path.expanduser('~/juju-dist'),
            'aws', remote_root='testing', dry_run=False, verbose=False)

    @patch('publish_streams.urllib2.urlopen', autospec=True)
    def test_get_remote_file(self, uo_mock):
        uo_mock.return_value = StringIO('data')
        content = get_remote_file('http://foo/bar.json')
        uo_mock.assert_called_with('http://foo/bar.json')
        self.assertEqual('data', content)

    @patch('publish_streams.get_remote_file', autospec=True)
    def test_diff_files(self, gr_mock):
        gr_mock.return_value = 'one\ntwo\nthree'
        with temp_dir() as base:
            local_path = os.path.join(base, 'bar.json')
            with open(local_path, 'w') as local_file:
                local_file.write('one\ntwo\nthree')
            identical, diff = diff_files(local_path, 'http://foo/bar.json')
        self.assertTrue(identical)
        self.assertIsNone(diff)
        gr_mock.assert_called_with('http://foo/bar.json')

    @patch('publish_streams.get_remote_file', autospec=True)
    def test_diff_files_different(self, gr_mock):
        gr_mock.return_value = 'one\ntwo\nfour'
        with temp_dir() as base:
            local_path = os.path.join(base, 'bar.json')
            with open(local_path, 'w') as local_file:
                local_file.write('one\ntwo\nthree')
            identical, diff = diff_files(local_path, 'http://foo/bar.json')
        self.assertFalse(identical)
        normalized_diff = re.sub('/tmp/.*/bar', '/tmp/bar', diff)
        self.assertEqual(
            '--- /tmp/bar.json\n\n'
            '+++ http://foo/bar.json\n\n'
            '@@ -1,3 +1,3 @@\n\n'
            ' one\n'
            ' two\n'
            '-three\n'
            '+four',
            normalized_diff)

    @patch('publish_streams.diff_files', autospec=True)
    def test_verify_metadata(self, df_mock):
        df_mock.return_value = (True, None)
        with temp_dir() as base:
            metadata_path = os.path.join(base, 'tools', 'streams', 'v1')
            os.makedirs(metadata_path)
            metadata_file = os.path.join(metadata_path, 'bar.json')
            metadasta_sig = os.path.join(metadata_path, 'bar.json.sig')
            with open(metadata_file, 'w') as local_file:
                local_file.write('bar.json')
            with open(metadasta_sig, 'w') as local_file:
                local_file.write('bar.json.sig')
            identical, diff = verify_metadata(base, CPCS['aws'], verbose=False)
        self.assertTrue(identical)
        self.assertIsNone(diff)
        self.assertEqual(1, df_mock.call_count)
        df_mock.assert_called_with(
            metadata_file, '{}/tools/streams/v1/bar.json'.format(CPCS['aws']))

    @patch('publish_streams.diff_files', autospec=True)
    def test_verify_metadata_with_root_faile(self, df_mock):
        df_mock.return_value = (False, 'different')
        with temp_dir() as base:
            metadata_path = os.path.join(base, 'tools', 'streams', 'v1')
            os.makedirs(metadata_path)
            metadata_file = os.path.join(metadata_path, 'bar.json')
            with open(metadata_file, 'w') as local_file:
                local_file.write('bar.json')
            identical, diff = verify_metadata(base, CPCS['aws'])
        self.assertFalse(identical)
        self.assertEqual(diff, 'different')
        df_mock.assert_called_with(
            metadata_file, '{}/tools/streams/v1/bar.json'.format(CPCS['aws']))

    @patch('publish_streams.verify_metadata', autospec=True)
    def test_publish(self, vm_mock):
        vm_mock.return_value = (True, None)
        publish('testing', '/streams/juju-dist', 'aws',
                remote_root=None, dry_run=False, verbose=False)
        vm_mock.assert_called_with(
            '/streams/juju-dist', CPCS['aws'], verbose=False)

    @patch('publish_streams.verify_metadata', autospec=True)
    def test_publish_with_remote(self, vm_mock):
        vm_mock.return_value = (True, None)
        publish('testing', '/streams/juju-dist', 'aws',
                remote_root='weekly', dry_run=False, verbose=False)
        vm_mock.assert_called_with(
            '/streams/juju-dist', '%s/weekly' % CPCS['aws'], verbose=False)
