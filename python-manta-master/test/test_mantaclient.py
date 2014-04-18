#!/usr/bin/env python
# Copyright (c) 2012 Joyent, Inc.  All rights reserved.

"""Test the python-manta MantaClient."""

import os
import sys
import re
from posixpath import dirname as udirname, basename as ubasename, join as ujoin
from pprint import pprint
import unittest
import codecs

from testlib import TestError, TestSkipped, tag

from common import *
import manta



#---- globals

TDIR = "tmp/test_mantaclient"



#---- internal support stuff


#---- Test cases
#
# We need to run these tests in order. We'll be creating a test area:
#   /$account/stor/tmp/test_mantaclient/
# and working in there.
#

class MiscTestCase(MantaTestCase):
    """Miscellaneous 'manta' module tests."""
    def test_imports(self):
        self.assertTrue(manta.MantaClient)
        self.assertTrue(manta.PrivateKeySigner)
        self.assertTrue(manta.SSHAgentSigner)
        self.assertTrue(manta.MantaError)
        self.assertTrue(manta.MantaAPIError)

    def test_version(self):
        VERSION_RE = re.compile('^\d+\.\d+\.\d+$')
        self.assertTrue(manta.__version__)
        self.assertTrue(VERSION_RE.search(manta.__version__))

class CleanTestAreaTestCase(MantaTestCase):
    def test_clean(self):
        client = self.get_client()
        try:
            client.list_directory(stor(TDIR))
        except manta.MantaError, ex:
            if ex.code == "ResourceNotFound":
                return
            else:
                raise
        # Don't totally wipe, to save time on test re-runs... though
        # I'm sure this will surprise at some point.
        skips = [stor(TDIR), stor(TDIR, 'manyfiles')]
        for mdir, dirs, nondirs in client.walk(stor(TDIR), False):
            if mdir in skips:
                continue
            for nondir in nondirs:
                client.delete_object(ujoin(mdir, nondir["name"]))
            client.delete_object(mdir)

class DirTestCase(MantaTestCase):
    def test_put(self):
        client = self.get_client()
        client.mkdirp(stor(TDIR))
        dirents = client.list_directory(stor(udirname(TDIR)))
        dirent = [d for d in dirents if d["name"] == ubasename(TDIR)][0]
        self.assertTrue(dirent)

    def test_listheaddel(self):
        client = self.get_client()
        client.mkdirp(stor(TDIR, 'dir'))
        for d in ['a', 'b', 'c']:
            client.mkdirp(stor(TDIR, 'dir', d))
        dirents = client.list_directory(stor(TDIR, 'dir'))
        self.assertEqual(len(dirents), 3,
            'unexpected number of dirents: got %d, expected 3, dirents %r' % (
                len(dirents), dirents))
        dirents = client.list_directory(stor(TDIR, 'dir'), limit=2)
        self.assertEqual(len(dirents), 2)
        dirents = client.list_directory(stor(TDIR, 'dir'), marker=dirents[-1]["name"])
        self.assertEqual(len(dirents), 2)
        self.assertEqual(dirents[1]["name"], "c")

        res = client.head_directory(stor(TDIR, 'dir'))
        self.assertEqual(int(res['result-set-size']), 3)

        for d in ['a', 'b', 'c']:
            client.delete_directory(stor(TDIR, 'dir', d))
        dirents = client.list_directory(stor(TDIR, 'dir'))
        self.assertEqual(len(dirents), 0)

class ObjectTestCase(MantaTestCase):
    def test_putgetdel(self):
        client = self.get_client()
        client.mkdirp(stor(TDIR))
        mpath = stor(TDIR, 'foo.txt')
        content = 'foo\nbar\nbaz'
        client.put_object(mpath, content=content)
        got = client.get_object(mpath)
        self.assertEqual(content, got)
        client.delete_object(mpath)
        dirents = [e for e in client.list_directory(stor(TDIR))
            if e["name"] == "foo.txt"]
        self.assertEqual(len(dirents), 0)

class LinkTestCase(MantaTestCase):
    def test_put(self):
        client = self.get_client()
        client.mkdirp(stor(TDIR))
        obj_path = stor(TDIR, 'obj.txt')
        content = 'foo\nbar\nbaz'
        client.put_object(obj_path, content=content)
        link_path = stor(TDIR, 'link.txt')
        client.put_snaplink(link_path, obj_path)
        got = client.get_object(link_path)
        self.assertEqual(content, got)
        client.delete_object(obj_path)
        got2 = client.get_object(link_path)
        self.assertEqual(content, got2)
        client.delete_object(link_path)
        dirents = [e for e in client.list_directory(stor(TDIR))
            if e["name"] in ("obj.txt", "link.txt")]
        self.assertEqual(len(dirents), 0)


class ManyFilesTestCase(MantaTestCase):
    __tags__ = ['slow']

    def setUp(self):
        self.client = self.get_client()
        self.base = b = ujoin(TDIR, "manyfiles")
        # If this dir exists already, then save time, don't rebuild it (i.e.
        # presuming all the files were left in place).
        if self.client.type(stor(b)) != "directory":
            self.client.mkdirp(stor(b))
            for i in range(1100):
                self.client.put(stor(b, "f%05d" % i), "index %d" % i)

    def test_count(self):
        ls = self.client.ls(stor(self.base))
        self.assertEqual(len(ls), 1100)
