#!/usr/bin/env python

# Copyright 2013 Canonical Ltd.

import os
import sys
from optparse import OptionParser

from jujuman import JujuMan


GENERATORS = {
    'man': JujuMan
}

# Insert the directory that this module is in into the python path.
sys.path.insert(0, (os.path.dirname(__file__)))

def main(argv):
    parser = OptionParser(usage="""%prog [options] OUTPUT_FORMAT

Available OUTPUT_FORMAT:

    man              man page

And that is all for now.""")

    parser.add_option("-s", "--show-filename",
                      action="store_true", dest="show_filename", default=False,
                      help="print default filename on stdout")

    parser.add_option("-o", "--output", dest="filename", metavar="FILE",
                      help="write output to FILE")

    (options, args) = parser.parse_args(argv)

    if len(args) != 2:
        parser.print_help()
        sys.exit(1)

    try:
        doc_generator = GENERATORS[args[1]]()
    except KeyError as e:
        sys.stderr.write("Unknown documentation generator %r\n" % e.message)
        sys.exit(1)

    if options.filename:
        outfilename = options.filename
    else:
        outfilename = doc_generator.get_filename(options)

    if outfilename == "-":
        outfile = sys.stdout
    else:
        outfile = open(outfilename, "w")
    if options.show_filename and (outfilename != "-"):
        sys.stdout.write(outfilename)
        sys.stdout.write('\n')

    doc_generator.write_documentation(options, outfile)


if __name__ == "__main__":
    main(sys.argv)
