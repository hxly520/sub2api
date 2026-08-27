#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
classifier="${script_dir}/classify-update.sh"
tag_validator="${script_dir}/validate-private-tag.sh"
workflow="${script_dir}/../../.github/workflows/release.yml"
fixture="$(mktemp -d)"
trap 'rm -rf "${fixture}"' EXIT

export RUNNER_TEMP="${fixture}/runner-temp"
mkdir -p "${RUNNER_TEMP}"
cd "${fixture}"

git init --quiet
git config user.name "Release Fixture"
git config user.email "release-fixture@example.invalid"
git config core.autocrlf false

commit_file() {
  local path="$1"
  local content="$2"
  mkdir -p "$(dirname "${path}")"
  printf '%s\n' "${content}" > "${path}"
  git add "${path}"
  git commit --quiet -m "fixture: ${path}"
}

tag_release() {
  local tag="$1"
  local message="${2:-fixture release}"
  git tag -a "${tag}" -m "${message}"
}

assert_policy() {
  local expected="$1"
  local current="$2"
  local previous="${3:-}"
  local manifest
  manifest="$(bash "${classifier}" "${current}" "${previous}")"
  jq -e --arg expected "${expected}" '.schema_version == 1 and .policy == $expected' <<<"${manifest}" >/dev/null
  jq -e --arg version "${current#v}" '.version == $version and (.source_commit | length == 40)' <<<"${manifest}" >/dev/null
}

grep -Fq 'bash tools/release/validate-private-tag.sh "${tag}"' "${workflow}"
grep -Fq 'needs: validate-release-source' "${workflow}"
grep -Fq 'git merge-base --is-ancestor "${DEFAULT_COMMIT}" "${TAG_COMMIT}"' "${workflow}"
grep -Fq 'git merge-base --is-ancestor "${TAG_COMMIT}" "${DEFAULT_COMMIT}"' "${workflow}"
grep -Fq 'refusing an old-tag rerun or VERSION downgrade' "${workflow}"
grep -Fq 'Release tag ${TAG_NAME} contains VERSION=${SOURCE_VERSION}, expected ${VERSION}.' "${workflow}"
if grep -Fq 'sync-version-file:' "${workflow}" || grep -Fq 'git push origin "HEAD:refs/heads/${DEFAULT_BRANCH}"' "${workflow}"; then
  echo "release workflow mutates the production-tracking default branch" >&2
  exit 1
fi
if grep -Fq 'git fetch --tags --force' "${workflow}"; then
  echo "release workflow force-fetches mutable tags" >&2
  exit 1
fi
bash "${tag_validator}" "v0.1.173-52t.1"
for invalid_tag in \
  "v01.1.173-52t.1" \
  "v0.01.173-52t.1" \
  "v0.1.0173-52t.1" \
  "v0.1.173-52t.01"; do
  if bash "${tag_validator}" "${invalid_tag}" >/dev/null 2>&1; then
    echo "tag validator accepted non-canonical SemVer: ${invalid_tag}" >&2
    exit 1
  fi
done

commit_file "backend/main.go" "package main"
tag_release "v0.1.173-52t.1"
assert_policy "image-update-required" "v0.1.173-52t.1"

commit_file "backend/feature.go" "package main"
tag_release "v0.1.173-52t.2"
assert_policy "hot-update-safe" "v0.1.173-52t.2" "v0.1.173-52t.1"

commit_file "backend/migrations/999_fixture.sql" "SELECT 1;"
tag_release "v0.1.173-52t.3"
assert_policy "image-update-required" "v0.1.173-52t.3" "v0.1.173-52t.2"

commit_file "backend/migrations/1000_fixture.sql" "SELECT 2;"
tag_release "v0.1.173-52t.4" "[migration-hot-update-safe] backward-compatible fixture"
assert_policy "hot-update-safe" "v0.1.173-52t.4" "v0.1.173-52t.3"

commit_file "Dockerfile" "FROM scratch"
tag_release "v0.1.173-52t.5"
assert_policy "image-update-required" "v0.1.173-52t.5" "v0.1.173-52t.4"

commit_file "frontend/feature.ts" "export const fixture = true"
tag_release "v0.1.173-52t.6" "[image-update-recommended] fixture"
assert_policy "image-update-recommended" "v0.1.173-52t.6" "v0.1.173-52t.5"

git checkout --quiet -b divergent "v0.1.173-52t.2"
commit_file "backend/divergent.go" "package main"
tag_release "v0.1.173-52t.7"
assert_policy "image-update-required" "v0.1.173-52t.7" "v0.1.173-52t.6"

if bash "${classifier}" "v0.1.173" >/dev/null 2>&1; then
  echo "invalid private tag was accepted" >&2
  exit 1
fi
git tag "v0.1.173-52t.8"
if bash "${classifier}" "v0.1.173-52t.8" "v0.1.173-52t.7" >/dev/null 2>&1; then
  echo "lightweight private tag was accepted" >&2
  exit 1
fi

echo "classify-update fixtures passed"
