#!/usr/bin/env bash
set -euo pipefail

current_tag="${1:?usage: classify-update.sh <vX.Y.Z-52t.N> [previous-tag]}"
previous_tag="${2:-}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tag_validator="${script_dir}/validate-private-tag.sh"

bash "${tag_validator}" "${current_tag}"

git rev-parse --verify "${current_tag}^{commit}" >/dev/null
if [[ "$(git cat-file -t "${current_tag}")" != "tag" ]]; then
  echo "private releases require an annotated tag: ${current_tag}" >&2
  exit 2
fi
if [[ -z "${previous_tag}" ]]; then
  previous_tag="$(git tag --list 'v*-52t.*' --sort=-version:refname | grep -Fvx "${current_tag}" | head -n 1 || true)"
fi
if [[ -n "${previous_tag}" ]]; then
  bash "${tag_validator}" "${previous_tag}"
fi

policy="hot-update-safe"
declare -a reasons=()
declare -a changed_files=()

if [[ -z "${previous_tag}" ]]; then
  policy="image-update-required"
  reasons+=("first private release establishes the image and updater baseline")
else
  git rev-parse --verify "${previous_tag}^{commit}" >/dev/null
  if [[ "$(git cat-file -t "${previous_tag}")" != "tag" ]]; then
    echo "previous private release must be an annotated tag: ${previous_tag}" >&2
    exit 2
  fi
  if ! git merge-base --is-ancestor "${previous_tag}^{commit}" "${current_tag}^{commit}"; then
    policy="image-update-required"
    reasons+=("previous private release is not an ancestor of the current release")
  fi
  while IFS= read -r path; do
    [[ -n "${path}" ]] && changed_files+=("${path}")
  done < <(git diff --name-only "${previous_tag}^{commit}" "${current_tag}^{commit}")
fi

tag_message="$(git for-each-ref "refs/tags/${current_tag}" --format='%(contents)')"
migration_reviewed=false
if grep -Fq '[migration-hot-update-safe]' <<<"${tag_message}"; then
  migration_reviewed=true
fi

for path in "${changed_files[@]}"; do
  case "${path}" in
    Dockerfile|Dockerfile.*|deploy/Dockerfile|deploy/docker-entrypoint.sh|deploy/docker-compose*.yml|deploy/nginx/*|deploy/public-landing/*|backend/resources/*|points-system/*)
      policy="image-update-required"
      reasons+=("runtime or independently deployed file changed: ${path}")
      ;;
    backend/migrations/*)
      if [[ "${migration_reviewed}" != true ]]; then
        policy="image-update-required"
        reasons+=("database migration requires explicit [migration-hot-update-safe] review: ${path}")
      else
        reasons+=("reviewed backward-compatible database migration: ${path}")
      fi
      ;;
  esac
done

if grep -Fq '[image-update-required]' <<<"${tag_message}"; then
  policy="image-update-required"
  reasons+=("release tag explicitly requires an image update")
elif [[ "${policy}" == "hot-update-safe" ]] && grep -Fq '[image-update-recommended]' <<<"${tag_message}"; then
  policy="image-update-recommended"
  reasons+=("release tag recommends synchronizing the image")
fi

source_commit="$(git rev-parse "${current_tag}^{commit}")"
generated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

printf '%s\n' "${changed_files[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))' > "${RUNNER_TEMP:-/tmp}/sub2api-changed-files.json"
printf '%s\n' "${reasons[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))' > "${RUNNER_TEMP:-/tmp}/sub2api-update-reasons.json"

jq -n \
  --arg version "${current_tag#v}" \
  --arg source_commit "${source_commit}" \
  --arg policy "${policy}" \
  --arg previous_version "${previous_tag#v}" \
  --arg generated_at "${generated_at}" \
  --slurpfile reasons "${RUNNER_TEMP:-/tmp}/sub2api-update-reasons.json" \
  --slurpfile changed_files "${RUNNER_TEMP:-/tmp}/sub2api-changed-files.json" \
  '{
    schema_version: 1,
    version: $version,
    source_commit: $source_commit,
    policy: $policy,
    reasons: $reasons[0],
    previous_version: $previous_version,
    generated_at: $generated_at,
    changed_files: $changed_files[0]
  }'
