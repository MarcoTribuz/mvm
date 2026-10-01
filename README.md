# mvm — Meteor Version Manager

Install several [Meteor](https://www.meteor.com) releases side by side and run
each command with the right one. Built for CI agents (Jenkins, GitHub Actions,
GitLab) that build many apps pinned to different Meteor versions, and handy on
dev machines too.

- **Per-project, per-process**: the release comes from `.meteor/release`, so
  parallel builds on the same agent can use different versions. No global
  switch, no race.
- **Download once**: releases live in `~/.mvm` (or `$MVM_HOME`, e.g. a Docker
  volume or K8s PVC). Concurrent builds share one download through a file lock.
- **No Node conflicts**: every Meteor release ships its own Node; `mvm exec`
  puts that node/npm on `PATH` (Node 14 for 2.x, 20+ for 3.x).
- **Single static binary**, no dependencies. Linux and macOS, x86_64 and arm64.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/MarcoTribuz/mvm/main/install.sh | sh
```

The installer puts `mvm` in `~/.local/bin` (`MVM_INSTALL_DIR`), creates the
`meteor` shim and adds a block with `PATH` and shell completion to your
profile (`~/.zshrc`, `~/.bashrc`/`~/.bash_profile` or fish `config.fish`).
Choose the file with `PROFILE=~/.myrc`, or skip it with `PROFILE=/dev/null`:

```sh
curl -fsSL https://raw.githubusercontent.com/MarcoTribuz/mvm/main/install.sh | PROFILE=/dev/null sh
```

Update later with `mvm self-update` (or `mvm self-update <version>`).

## Uninstall

Removes the mvm binary, `$MVM_HOME` (downloaded releases and the shim) and
the profile block added by the installer; `~/.meteor` is left alone. Asks for
confirmation; pass `-y` to skip it.

```sh
curl -fsSL https://raw.githubusercontent.com/MarcoTribuz/mvm/main/uninstall.sh | sh
```

## Usage

```sh
mvm ls-remote 3              # available 3.x releases
mvm install 2.16 3.3         # install releases
mvm list                     # installed releases (-> = used here)

cd my-app                    # contains .meteor/release = METEOR@3.3
mvm current                  # 3.3 (from /…/my-app/.meteor/release, installed)
mvm exec -- meteor npm ci
mvm exec -- meteor build ../out --server-only
mvm exec --auto-install -- meteor test --once --driver-package meteortesting:mocha

mvm exec 2.16 -- meteor create legacy-app   # explicit release
mvm use 3.3                  # default outside projects
eval "$(mvm env)"            # put the resolved release on PATH in this shell

mvm prune --unused-days 30   # housekeeping
mvm doctor                   # check the setup
```

### Transparent `meteor` command

```sh
mvm init   # creates $MVM_HOME/bin/meteor and prints: export PATH=…/.mvm/bin:"$PATH"
```

With that directory on `PATH`, plain `meteor …` runs the release the current
directory needs (set `MVM_AUTO_INSTALL=1` to install missing releases on the fly).

### Aliases

```sh
mvm alias legacy 2.16      # name a release
mvm exec legacy -- meteor --version
mvm install latest         # built-in: newest stable release for this platform
mvm alias                  # list (also shown in `mvm list`)
mvm unalias legacy
```

Aliases work wherever a version does: arguments, `MVM_METEOR_VERSION` and
`.mvmrc`. `default` is the release set with `mvm use`. `.meteor/release`
always names a real release.

### Shell completion

The installer sets it up. To do it by hand, add one of these to your profile:

```sh
source <(mvm completion zsh)       # zsh (after compinit)
eval "$(mvm completion bash)"      # bash
mvm completion fish | source       # fish
```

### Version resolution order

1. explicit version (`mvm exec 3.1 -- …`)
2. `MVM_METEOR_VERSION`
3. `.meteor/release` in the current directory or a parent
4. `.mvmrc` in the current directory or a parent
5. default set with `mvm use`

## Configuration

| Variable | Purpose |
|---|---|
| `MVM_HOME` | store location (default `~/.mvm`) |
| `MVM_AUTO_INSTALL=1` | `exec` and the shim install missing releases |
| `MVM_MIRROR` | tarball base URL (Artifactory/Nexus/S3), layout `<base>/<version>/meteor-bootstrap-os.<os>.<arch>.tar.gz` (`os.linux.x86_64`, `os.linux.aarch64`, `os.osx.x86_64`, `os.osx.arm64`) |
| `MVM_KEEP_DOWNLOADS=1` | keep tarballs in `$MVM_HOME/cache/downloads` |
| `GITHUB_TOKEN` | avoids GitHub rate limits for `ls-remote` |

## CI

See [docs/jenkins.md](docs/jenkins.md) for Jenkins pipelines on static VMs,
Docker and Kubernetes agents.

## How it works

Each release is the official bootstrap tarball (the one `install.meteor.com`
uses) unpacked into its own warehouse, `$MVM_HOME/versions/<version>/.meteor`.
`mvm exec` runs commands with `METEOR_WAREHOUSE_DIR` pointing at that warehouse
and its `meteor` and bundled `node` first on `PATH`. `~/.meteor` is never
touched. See [docs/adr/0001-isolation.md](docs/adr/0001-isolation.md).

Downloads are verified against the SHA-256 recorded the first time a release
was fetched (Meteor does not publish checksums).

## Supported releases

| | Linux x86_64 | Linux arm64 | macOS x86_64 | macOS arm64 |
|---|---|---|---|---|
| Meteor 3.x | ✓ | ✓ | ✓ | ✓ |
| Meteor 2.x | ✓ | – (not published by Meteor) | ✓ | ✓ |

On Apple Silicon, `MVM_ARCH=x86_64` installs the Intel build to run under Rosetta.

## License

MIT
