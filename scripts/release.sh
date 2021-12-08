#!/usr/bin/env bash

gox -output="../bin/release/temp/{{.OS}}_{{.Arch}}/{{.Dir}}" ../cmd/protoc-gen-bean

for d in ../bin/release/temp/*; do
    for bin in ${d}; do
        zip_name=$(basename ${bin})
        for f in ${bin}/*; do
            zip -j -X ../bin/release/${zip_name}.zip ${f}
        done
    done
done
