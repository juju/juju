import datetime
import json
from mock import patch
import os
from unittest import TestCase

from utils import temp_dir
from generate_mirrors import (
    generate_mirrors_file,
    generate_cpc_mirrors_file,
    get_deprecated_mirror,
    main,
    PURPOSES,
)


class GenerateMirrors(TestCase):

    def test_get_deprecated_mirror(self):
        self.assertEqual(
            None, get_deprecated_mirror('juju-dist/tools/streams/v1'))
        self.assertEqual(
            'devel', get_deprecated_mirror('juju-dist/devel/tools/streams/v1'))
        self.assertEqual(
            'proposed',
            get_deprecated_mirror('juju-dist/proposed/tools/streams/v1'))

    def test_generate_mirrors_file(self):
        updated = datetime.datetime.utcnow()
        with temp_dir() as base_path:
            stream_path = '%s/tools/streams/v1' % base_path
            os.makedirs(stream_path)
            generate_mirrors_file(updated, stream_path)
            mirror_path = '%s/mirrors.json' % stream_path
            self.assertTrue(os.path.isfile(mirror_path))
            with open(mirror_path) as mirror_file:
                data = json.load(mirror_file)
        self.assertEqual(['mirrors'], data.keys())
        expected_produts = sorted(
            'com.ubuntu.juju:%s:tools' % p for p in PURPOSES)
        self.assertEqual(expected_produts, sorted(data['mirrors'].keys()))
        first_purpose = data['mirrors']['com.ubuntu.juju:released:tools'][0]
        self.assertEqual('content-download', first_purpose['datatype'])
        self.assertEqual('mirrors:1.0', first_purpose['format'])
        self.assertEqual('streams/v1/cpc-mirrors.json', first_purpose['path'])
        expected_updated = updated.strftime('%Y%m%d')
        self.assertEqual(expected_updated, first_purpose['updated'])

    def test_cpc_generate_mirrors_file(self):
        updated = datetime.datetime.utcnow()
        with temp_dir() as base_path:
            stream_path = '%s/tools/streams/v1' % base_path
            os.makedirs(stream_path)
            generate_cpc_mirrors_file(updated, stream_path)
            mirror_path = '%s/cpc-mirrors.json' % stream_path
            self.assertTrue(os.path.isfile(mirror_path))
            with open(mirror_path) as mirror_file:
                data = json.load(mirror_file)
        self.assertEqual(['format', 'mirrors', 'updated'], sorted(data.keys()))
        self.assertEqual('mirrors:1.0', data['format'])
        expected_updated = updated.strftime('%a, %d %b %Y %H:%M:%S -0000')
        self.assertEqual(expected_updated, data['updated'])
        expected_produts = sorted(
            'com.ubuntu.juju:%s:tools' % p for p in PURPOSES)
        for product_name in expected_produts:
            purposeful_mirrors = data['mirrors'][product_name]
            purpose = product_name.split(':')[1]
            self.assertEqual(
                'streams/v1/com.ubuntu.juju-%s-tools.json' % purpose,
                purposeful_mirrors[0]['path'])
            self.assertEqual(
                'https://juju-dist.s3.amazonaws.com/tools',
                purposeful_mirrors[0]['mirror'])
            self.assertEqual(11, len(purposeful_mirrors[0]['clouds']))
            self.assertEqual(
                'https://jujutools.blob.core.windows.net/juju-tools/tools',
                purposeful_mirrors[1]['mirror'])
            self.assertEqual(17, len(purposeful_mirrors[1]['clouds']))
            self.assertEqual(
                ("https://us-east.manta.joyent.com/"
                 "cpcjoyentsupport/public/juju-dist/tools"),
                purposeful_mirrors[2]['mirror'])
            self.assertEqual(6, len(purposeful_mirrors[2]['clouds']))

    def test_cpc_generate_mirrors_file_deprecated_tree(self):
        updated = datetime.datetime.utcnow()
        with temp_dir() as base_path:
            stream_path = '%s/devel/tools/streams/v1' % base_path
            os.makedirs(stream_path)
            generate_cpc_mirrors_file(updated, stream_path)
            mirror_path = '%s/cpc-mirrors.json' % stream_path
            self.assertTrue(os.path.isfile(mirror_path))
            with open(mirror_path) as mirror_file:
                data = json.load(mirror_file)
        product_name = 'com.ubuntu.juju:released:tools'
        self.assertEqual([product_name], data['mirrors'].keys())
        purposeful_mirror = data['mirrors'][product_name]
        purpose = product_name.split(':')[1]
        self.assertEqual(
            'streams/v1/com.ubuntu.juju-%s-tools.json' % purpose,
            purposeful_mirror[0]['path'])
        self.assertEqual(
            'https://juju-dist.s3.amazonaws.com/devel/tools',
            purposeful_mirror[0]['mirror'])
        self.assertEqual(11, len(purposeful_mirror[0]['clouds']))
        self.assertEqual(
            'https://jujutools.blob.core.windows.net'
            '/juju-tools/devel/tools',
            purposeful_mirror[1]['mirror'])
        self.assertEqual(17, len(purposeful_mirror[1]['clouds']))
        self.assertEqual(
            ("https://us-east.manta.joyent.com/"
             "cpcjoyentsupport/public/juju-dist/devel/tools"),
            purposeful_mirror[2]['mirror'])
        self.assertEqual(6, len(purposeful_mirror[2]['clouds']))

    def test_main(self):
        with patch('generate_mirrors.generate_mirrors_file') as gmf_mock:
            with patch(
                    'generate_mirrors.generate_cpc_mirrors_file') as gcmf_mock:
                main(['streams/juju-dist/tools'])
                for mock in (gmf_mock, gcmf_mock):
                    args, kwargs = mock.call_args
                    self.assertIsInstance(args[0], datetime.datetime)
                    self.assertEqual(
                        'streams/juju-dist/tools/streams/v1', args[1])
                    self.assertFalse(kwargs['verbose'])
                    self.assertFalse(kwargs['dry_run'])
