#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: $0 BOOK.epub" >&2
  exit 2
fi

book=$1
version=5.3.0
archive_sha256=6c07e68584b2e2ce2f89fe06e1246dfead3eb36b46b340e7d93524f29dcff6c5
cache_root=${XDG_CACHE_HOME:-"$HOME/.cache"}/bookset
archive=$cache_root/epubcheck-$version.zip
jar=$cache_root/epubcheck-$version/epubcheck.jar

if [ ! -f "$jar" ]; then
  mkdir -p "$cache_root"
  curl --fail --location --silent --show-error \
    "https://github.com/w3c/epubcheck/releases/download/v$version/epubcheck-$version.zip" \
    --output "$archive"
  actual_sha256=$(shasum -a 256 "$archive" | awk '{print $1}')
  if [ "$actual_sha256" != "$archive_sha256" ]; then
    echo "epubcheck archive checksum mismatch" >&2
    exit 1
  fi
  unzip -q "$archive" -d "$cache_root"
fi

exec java -jar "$jar" "$book"
