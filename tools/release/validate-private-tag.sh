#!/usr/bin/env bash
set -euo pipefail

tag="${1:?usage: validate-private-tag.sh <vX.Y.Z-52t.N>}"
private_tag_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-52t\.([1-9][0-9]*)$'

if [[ ! "${tag}" =~ ${private_tag_pattern} ]]; then
  echo "private release tag must match canonical vX.Y.Z-52t.N SemVer: ${tag}" >&2
  exit 2
fi
