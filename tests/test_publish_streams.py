from mock import patch
import os
import re
from StringIO import StringIO
from unittest import TestCase

from publish_streams import (
    diff_files,
    get_remote_file,
    main,
    parse_args,
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
