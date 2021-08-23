#!/usr/bin/env bash

set -e
set -o pipefail

WD="$(pwd)"

mkdir -p "$WD/bin/"

# install latest from main branch
TMP_DIR="$(mktemp -d)"
cd "$TMP_DIR"
git clone https://github.com/lance/func
cd func
git checkout lance/builtin-language-packs
echo "Downloading pkger"
go get github.com/markbates/pkger/cmd/pkger
make
cp func "$WD/bin/func_snapshot"
cd "$WD"
rm -fr "$TMP_DIR"
