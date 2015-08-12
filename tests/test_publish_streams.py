from mock import patch
import os
from unittest import TestCase

from publish_streams import (
    main,
    parse_args,
)


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
