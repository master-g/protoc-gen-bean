#!/usr/bin/env python
# -*- coding: utf-8 -*-

# fun stuff

import sys
import os
import os.path
import fnmatch
import time
import shutil
import fileinput


class bcolors:
    HEADER = '\033[95m'
    OKBLUE = '\033[94m'
    OKGREEN = '\033[92m'
    WARNING = '\033[93m'
    FAIL = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'
    UNDERLINE = '\033[4m'


def printBold(*args):
    print(bcolors.BOLD + "".join(map(str, args)) + bcolors.ENDC)


def printHeader(*args):
    print(bcolors.HEADER + "".join(map(str, args)) + bcolors.ENDC)


def printBule(*args):
    print(bcolors.OKBLUE + "".join(map(str, args)) + bcolors.ENDC)


def printGreen(*args):
    print(bcolors.OKGREEN + "".join(map(str, args)) + bcolors.ENDC)


def printWarning(*args):
    print(bcolors.WARNING + "".join(map(str, args)) + bcolors.ENDC)


def printFail(*args):
    print(bcolors.FAIL + "".join(map(str, args)) + bcolors.ENDC)


def printUnderline(*args):
    print(bcolors.UNDERLINE + "".join(map(str, args)) + bcolors.ENDC)


def findAllFileWithName(modulePath, name):
    result = []
    for root, dirs, files in os.walk(modulePath):
        if name in files:
            result.append(os.path.join(root, name))
    return result


def findAllFileWithPattern(path, pattern):
    result = []
    for root, dirs, files in os.walk(path):
        for name in files:
            if fnmatch.fnmatch(name, pattern):
                result.append(os.path.join(root, name))
    return result


def safeGet(dict, setName):
    if setName in dict.keys():
        return dict[setName]
    else:
        return set()


def strTime():
    return time.strftime('%Y%m%d_%p%H%M%S', time.localtime())


def main(argv):
    printHeader('🐵  making output...')

    path = os.path.dirname(os.path.realpath(__file__))
    outputdir = path + os.path.sep + "output_" + strTime()

    if not os.path.exists(outputdir):
        os.makedirs(outputdir)

    protofiles = findAllFileWithPattern(path, "*.proto")
    for file in protofiles:
        newpath = outputdir + os.sep + os.path.basename(file)
        shutil.copyfile(file, newpath)
        flagadded = False
        for line in fileinput.FileInput(newpath, inplace=1):
            if 'package' in line and not flagadded:
                line = line + '\n' + 'option optimize_for = LITE_RUNTIME;\n'
                flagadded = True
            sys.stdout.write(line)

    javaoutput = os.path.join(outputdir, 'build')
    if not os.path.exists(javaoutput):
        os.makedirs(javaoutput)

    os.system('./protoc --java_out=' + javaoutput +
              ' -I=' + outputdir +
              ' ' + outputdir + os.path.sep + '*.proto')

    os.system('./protoc -I=' + outputdir +
              ' --plugin=protoc-gen-bean ' +
              '--bean_out=vopackage=vo,cvtpackage=protobuf.converter:' +
              javaoutput + ' ' + outputdir + os.path.sep + '*.proto')

    printBule('🍺  All done, have a nice day!')


if __name__ == "__main__":
    main(sys.argv[1:])
