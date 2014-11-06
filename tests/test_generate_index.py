import datetime
import json
from mock import patch
import os
from unittest import TestCase

from utils import temp_dir
from generate_index import (
    generate_index_file,
    main,
)


class GenerateIndex(TestCase):

    def test_generate_index_file(self):
        updated = datetime.datetime.utcnow()
        with temp_dir() as base_path:
            stream_path = '%s/tools/streams/v1' % base_path
            os.makedirs(stream_path)
            generate_index_file(updated, stream_path)
            index_path = '%s/index.json' % stream_path
            self.assertTrue(os.path.isfile(index_path))
            with open(index_path) as index_file:
                data = json.load(index_file)
        self.assertEqual(['format', 'index', 'updated'], sorted(data.keys()))
        self.assertEqual('index:1.0', data['format'])
        expected_updated = updated.strftime('%a, %d %b %Y %H:%M:%S -0000')
        self.assertEqual(expected_updated, data['updated'])
        product_name = 'com.ubuntu.juju:released:tools'
        self.assertEqual([product_name], data['index'].keys())
        released_index = data['index'][product_name]
        self.assertEqual(
            ['datatype', 'format', 'path', 'products', 'updated'],
            sorted(released_index.keys()))
        self.assertEqual('content-download', released_index['datatype'])
        self.assertEqual('products:1.0', released_index['format'])
        self.assertEqual(
            'streams/v1/com.ubuntu.juju:released:tools.json',
            released_index['path'])
        self.assertEqual(expected_updated, released_index['updated'])
        self.assertEqual(
            ['com.ubuntu.juju:12.04:amd64',
             'com.ubuntu.juju:12.04:armhf',
             'com.ubuntu.juju:12.04:i386',
             'com.ubuntu.juju:12.10:amd64',
             'com.ubuntu.juju:12.10:i386',
             'com.ubuntu.juju:13.04:amd64',
             'com.ubuntu.juju:13.04:i386',
             'com.ubuntu.juju:13.10:amd64',
             'com.ubuntu.juju:13.10:armhf',
             'com.ubuntu.juju:13.10:i386',
             'com.ubuntu.juju:14.04:amd64',
             'com.ubuntu.juju:14.04:arm64',
             'com.ubuntu.juju:14.04:armhf',
             'com.ubuntu.juju:14.04:i386',
             'com.ubuntu.juju:14.04:powerpc',
             'com.ubuntu.juju:14.04:ppc64',
             'com.ubuntu.juju:14.04:ppc64el',
             'com.ubuntu.juju:14.10:amd64',
             'com.ubuntu.juju:14.10:arm64',
             'com.ubuntu.juju:14.10:armhf',
             'com.ubuntu.juju:14.10:i386',
             'com.ubuntu.juju:14.10:ppc64',
             'com.ubuntu.juju:14.10:ppc64el',
             'com.ubuntu.juju:15.04:amd64',
             'com.ubuntu.juju:15.04:arm64',
             'com.ubuntu.juju:15.04:armhf',
             'com.ubuntu.juju:15.04:i386',
             'com.ubuntu.juju:15.04:ppc64',
             'com.ubuntu.juju:15.04:ppc64el'],
            released_index['products'])

    def test_main(self):
        with patch('generate_index.generate_index_file') as mock:
            main(['streams/juju-dist/tools'])
            args, kwargs = mock.call_args
            self.assertIsInstance(args[0], datetime.datetime)
            self.assertEqual(
                'streams/juju-dist/tools/streams/v1', args[1])
            self.assertFalse(kwargs['verbose'])
            self.assertFalse(kwargs['dry_run'])
