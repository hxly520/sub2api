# Release helper tools

The private release workflow uses these helpers to build the frontend once, build each target with GoReleaser OSS, verify all archives, build Linux images with Docker Buildx, and publish existing archives and checksums. All application build jobs use the same source commit peeled from the requested private annotated tag. Full releases produce five binary archives and GHCR linux/amd64 and linux/arm64 manifests. DockerHub is only used when its credentials are configured. Simple mode is amd64-only; it is not the default for private production releases.

The first job validates that the selected ref is an annotated `vX.Y.Z-52t.N` tag on the correct private main lineage and that its application tree has a matching `backend/cmd/server/VERSION`. The same exact tag must pass the quality and security workflows. Follow [the private release runbook](../../docs/PRIVATE_RELEASE_RUNBOOK_CN.md) before any real publication.

## Helper tests (no publication)

```bash
python -m pip install -r .github/release-tools/requirements-release.txt
python -m unittest discover -s .github/release-tools -p 'test_release_matrix.py'
bash -n .github/release-tools/release-images.sh
```

A manually dispatched `release.yml` run supports `dry_run=true` and still requires a valid existing private annotated tag. It builds local OCI archives and a check report without publishing or advancing main. A branch name does not bypass tag validation. Do not push a new matching tag just to test a build: any matching tag push triggers the real release workflow. Check that repository `SIMPLE_RELEASE` is not set to `true` when a full matrix is required.
