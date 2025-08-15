#!/usr/bin/python3

import sys
import argparse
from pathlib import Path
from html.parser import HTMLParser
from urllib.parse import urlsplit


class MetricsParser(HTMLParser):
    def __init__(self):
        super().__init__()
        self.int_link_count = 0
        self.ext_link_count = 0
        self.fragment_count = 0
        self.image_count = 0
        self.in_object = 0

    @property
    def link_count(self):
        return self.fragment_count + self.int_link_count + self.ext_link_count

    def read(self, file):
        """
        Read *file* (a file-like object with a ``read`` method returning
        strings) a chunk at a time, feeding each chunk to the parser.
        """
        # Ensure the parser state is reset before each file (just in case
        # there's an erroneous dangling <object>)
        self.reset()
        self.in_object = 0
        buf = ''
        while True:
            # Parse 1MB chunks at a time
            buf = file.read(1024**2)
            if not buf:
                break
            self.feed(buf)

    def handle_starttag(self, tag, attrs):
        """
        Count <a>, <img>, and <object> tags to determine the number of internal
        and external links, and the number of images.
        """
        attrs = dict(attrs)
        if tag == 'a' and 'href' in attrs:
            # If there's no href, it's an anchor; if there's no hostname
            # (netloc) or path, it's just a fragment link within the page
            url = urlsplit(attrs['href'])
            if url.netloc:
                self.ext_link_count += 1
            elif url.path:
                self.int_link_count += 1
            else:
                self.fragment_count += 1
        elif tag == 'object':
            # <object> tags are a bit complex as they nest to offer fallbacks
            # and may contain an <img> fallback. We only want to count the
            # outer-most <object> in this case
            if self.in_object == 0:
                self.image_count += 1
            self.in_object += 1
        elif tag == 'img' and self.in_object == 0:
            self.image_count += 1

    def handle_endtag(self, tag):
        if tag == 'object':
            # Never let in_object be negative
            self.in_object = max(0, self.in_object - 1)


def main(args=None):
    parser = argparse.ArgumentParser()
    parser.add_argument(
        'build_dir', metavar='build-dir', nargs='?', default='.',
        help="The directory to scan for HTML files")
    config = parser.parse_args(args)

    parser = MetricsParser()
    for path in Path(config.build_dir).rglob('*.html'):
        with path.open('r', encoding='utf-8', errors='replace') as f:
            parser.read(f)

    print('Summarising metrics for build files (.html)...')
    print(f'\tlinks: {parser.link_count} ('
          f'{parser.fragment_count} #frag…, '
          f'{parser.int_link_count} /int…, '
          f'{parser.ext_link_count} https://ext…'
          ')')
    print(f'\timages: {parser.image_count}')


if __name__ == '__main__':
    sys.exit(main())
