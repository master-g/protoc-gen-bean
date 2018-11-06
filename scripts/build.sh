#!/usr/bin/env bash

fastbuild=off
buildinfo=off
target=kos
while getopts fbt: opt
do
    case "$opt" in
      f)  fastbuild=on;;
      b)  buildinfo=on;;
      t)  target="$OPTARG";;
      \?)		# unknown flag
      	  echo >&2 \
	  "usage: $0 [-f] [-b] [-t target] [target ...]"
	  exit 1;;
    esac
done
shift `expr $OPTIND - 1`

if [ "$fastbuild" != on ]
then
    # format
    echo "==> Formatting..."
    goreturns -w $(find .. -type f -name '*.go' -not -path "../vendor/*")

    # mod
    echo "==> Module tidy and vendor..."
    go mod tidy
    go mod vendor

    # lint
    echo "==> Linting..."
    gometalinter	--vendor \
                    --fast \
                    --enable-gc \
                    --tests \
                    --aggregate \
                    --disable=gotype \
                    ../
fi

# build
echo "==> Building $target"

if [ "$version" != off ]
then
    PACKAGE=github.com/master-g/protoc-gen-bean/cmd/${target}
    COMMIT_HASH=$(git rev-parse --short HEAD)
    BUILD_DATE=$(date +%Y-%m-%dT%TZ%z)
    echo "==> Commit hash:$COMMIT_HASH Date:$BUILD_DATE"

    LD_FLAGS="-X ${PACKAGE}/buildinfo.CommitHash=${COMMIT_HASH} -X ${PACKAGE}/buildinfo.BuildDate=${BUILD_DATE}"

    # echo "${LD_FLAGS}"
    go build -ldflags "${LD_FLAGS}" -o ../bin/${target} ../cmd/${target}
else
    go build -o ../bin/${target} ../cmd/${target}
fi
