# ADR 0001 — One warehouse per release, selected with METEOR_WAREHOUSE_DIR

Status: accepted (2026-09-30, after the Phase 0 spike)

## Context

Meteor already keeps several `meteor-tool` versions in one warehouse
(`~/.meteor`) and "springboards" to the release in `.meteor/release`. That is
not enough for CI agents running builds in parallel:

- the shared `~/.meteor/meteor` symlink is rewritten by `meteor update` and
  the installer, so concurrent builds race;
- a `meteor` bootstrap must exist first, installed with a Node-version
  dependent npm package;
- nothing controls downloads, caching, checksums or cleanup.

## Decision

Each release is the official bootstrap tarball
(`static.meteor.com/packages-bootstrap/<v>/meteor-bootstrap-os.linux.<arch>.tar.gz`)
unpacked into its own warehouse `$MVM_HOME/versions/<v>/.meteor`. Commands run
with:

- `METEOR_WAREHOUSE_DIR=$MVM_HOME/versions/<v>/.meteor`
- `$MVM_HOME/versions/<v>/.meteor` (its `meteor` launcher) and the release's
  `dev_bundle/bin` (node/npm) first on `PATH`.

The version is resolved per process (argument → env → `.meteor/release` →
`.mvmrc` → default); no global state changes when switching.

mvm refuses meteor invocations that would springboard (see consequences):
running release A inside a project pinned to release B, and a bare
`meteor update`. Both are allowed when `--release` is passed.

The launcher symlink target (`.meteor/meteor`) is recorded at install time in
`versions/<v>/.mvm-launcher` and restored before every run.

## Spike results (Docker, linux/arm64 native and linux/amd64 emulated)

| Check | Result |
|---|---|
| Tarball URLs 2.16 / 3.0.4 / 3.3, x86_64 | 200 |
| Tarball URLs 3.0.4 / 3.3, aarch64 | 200 |
| Tarball URL 2.16, aarch64 | 403 (not published) |
| `meteor create --minimal` per release | ok, writes the right `METEOR@<v>` |
| `meteor build --server-only` 3.0.4 + 3.3 **in parallel** (arm64) | both ok, 22 s |
| `meteor build --server-only` 2.16 (amd64 under emulation) | ok, 50 s |
| Bundled node | 2.16 → v14.21.4, 3.0.4 → v20.18.0, 3.3 → v22.16.0 |
| `~/.meteor` created? | no |
| Disk per release (after create + build) | 1.9–2.9 GB |
| Release **mismatch** (3.3 tool in a 3.0.4 app) | infinite springboard loop, killed by timeout; also pulled meteor-tool 3.0.4 into the 3.3 warehouse |
| `meteor update --release 3.3` in the 3.3 warehouse | works, but re-points `.meteor/meteor` to the newest tool (3.5.2) and downloads it — `updateMeteorToolSymlink` always targets the latest default-track release |

## Consequences

- Parallel builds with different releases on one agent work; one install per
  release, shared by all builds.
- Uninstall is `rm -rf` of one directory; no cross-release package sharing, so
  disk use is higher than a single `~/.meteor` (packages are duplicated). Made
  acceptable by `mvm prune`; revisit if disk becomes an issue.
- The mismatch case hits meteor/meteor#14797 (fix proposed in
  meteor/meteor#14798). Until a fixed Meteor is widespread, mvm fails fast with
  a clear message instead of hanging. `meteor <cmd> --release X` does not
  springboard, so upgrades via `meteor update --release X` remain possible.
- Because `meteor update` re-points the launcher to the newest release, mvm
  pins it back on every run; otherwise the warehouse of release X would
  silently start running the newest tool. The newest tool downloaded by
  `meteor update` stays in that warehouse (disk only) until reinstall.
