#!/bin/bash
set -e
set -u
set -o pipefail

cd "$(dirname "${0}")/.."

# Setup temporary GOPATH so we can install go-bindata from vendor
export GOPATH=$( mktemp -d )
ln -s $( pwd )/vendor "${GOPATH}/src"
go install "./vendor/github.com/jteeuwen/go-bindata/..."

OUTDIR=${OUTDIR:-"."}
output="${OUTDIR}/pkg/operator/v311_00_assets/bindata.go"
${GOPATH}/bin/go-bindata \
    -nocompress \
    -nometadata \
    -prefix "bindata" \
    -pkg "v311_00_assets" \
    -o "${output}" \
    -ignore "OWNERS" \
    bindata/v3.11.0/...
gofmt -s -w "${output}"
