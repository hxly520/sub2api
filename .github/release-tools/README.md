# Release helper tools

These scripts are standalone utilities and are not called by `.github/workflows/release.yml`. The current release workflow uses GoReleaser and publishes a GitHub Release plus GHCR images after validation; it has no dry-run input.

Do not pass `dry_run` to `gh workflow run release.yml`: it is not a supported input and does not make that workflow safe for a trial run. Use the documented candidate branch, review, merge-to-main, and annotated-tag process in `docs/PRIVATE_RELEASE_RUNBOOK_CN.md` for a real release.

The helper tests validate release-matrix planning and command construction without contacting registries:

Helper checks:

```bash
python -m pip install -r .github/release-tools/requirements-release.txt
python -m unittest discover -s .github/release-tools -p 'test_release_matrix.py'
bash -n .github/release-tools/release-images.sh
```
