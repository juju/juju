import datetime
import json
from mock import patch
import os
from unittest import TestCase

from utils import temp_dir
from generate_mirrors import (
    generate_mirrors_file,
    generate_cpc_mirrors_file,
    PURPOSES,
)


class GenerateMirrors(TestCase):

    def test_generate_mirrors_file(self):
        updated = datetime.datetime.utcnow()
        with temp_dir() as base_path:
            stream_path = '%s/streams/v1' % base_path
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
            stream_path = '%s/streams/v1' % base_path
            os.makedirs(stream_path)
            generate_cpc_mirrors_file(updated, stream_path)
            mirror_path = '%s/cpc-mirrors.json' % stream_path
            self.assertTrue(os.path.isfile(mirror_path))
            with open(mirror_path) as mirror_file:
                data = json.load(mirror_file)
        self.assertEqual(['mirrors'], data.keys())
        self.assertEqual('mirrors:1.0', data['mirrors']['format'])
        expected_updated = updated.strftime('%a, %d %b %Y %H:%M:%S -0000')
        self.assertEqual(expected_updated, data['mirrors']['updated'])
        expected_produts = sorted(
            'com.ubuntu.juju:%s:tools' % p for p in PURPOSES)
        for product_name in expected_produts:
            purposeful_mirrors = data['mirrors'][product_name]
            purpose = product_name.split(':')[1]
            self.assertEqual(
                'streams/v1/com.ubuntu.juju:%s:tools.json' % purpose,
                purposeful_mirrors[0]['path'])
            self.assertEqual(
                'https://juju-dist.s3.amazonaws.com/tools',
                purposeful_mirrors[0]['mirror'])
            self.assertEqual(9, len(purposeful_mirrors[0]['clouds']))
            self.assertEqual(
                'https://jujutools.blob.core.windows.net/juju-tools/tools',
                purposeful_mirrors[1]['mirror'])
            self.assertEqual(17, len(purposeful_mirrors[1]['clouds']))
            self.assertEqual(
                ("https://region-a.geo-1.objects.hpcloudsvc.com/"
                 "v1/60502529753910/juju-dist/tools"),
                purposeful_mirrors[2]['mirror'])
            self.assertEqual(2, len(purposeful_mirrors[2]['clouds']))
            self.assertEqual(
                ("https://us-east.manta.joyent.com/"
                 "cpcjoyentsupport/public/juju-dist/tools"),
                purposeful_mirrors[3]['mirror'])
            self.assertEqual(6, len(purposeful_mirrors[3]['clouds']))
