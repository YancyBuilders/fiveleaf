# Contributing

## Development

```sh
scripts/dev.sh                                              # run FiveLeaf Dev with live reload, with its own settings
scripts/test.sh                                             # the tests, cmd/fiveleaf's over every core, as CI runs them
scripts/test.sh -full                                       # and the tests that start a shell
RUN=TestMerge scripts/test.sh                               # only cmd/fiveleaf's tests whose names match
go tool staticcheck ./...                                   # the lint CI runs, with gofmt -l .
go test ./...                                               # headless UI tests, one after another
go test -short ./...                                        # skip the tests that start a shell
go test -short -run XXX -bench Frame ./cmd/fiveleaf            # benchmark an editor frame
scripts/screenshots/shoot.sh                                # redraw the README screenshots
go run ./tools/icongen                                      # redraw the light and dark app icons and logos
```

`scripts/screenshots/shoot.sh` takes every feature, in dark and light, into `build/screenshots/raw`, frames them on
gradients in `build/screenshots/framed`, and puts the README's, 1600 pixels wide, in `docs/screenshots`. It needs
macOS, Python with Pillow and `pngquant`. To take one feature's shots, run
`FSHOTS=<dir> go test -run TestFeatColumns ./cmd/fiveleaf`.

## Releasing

Write the release notes under `## Unreleased` in `CHANGELOG.md`, as [AGENTS.md](AGENTS.md) describes, and commit them, then:

```sh
scripts/bump.sh patch    # or minor, major, or a version such as 0.2.0
```

The script sets the version in `mygo.json`, renames `## Unreleased` to `## <version>` in `CHANGELOG.md`, commits,
then pushes `main` and the `v<version>` tag. The [Release workflow](.github/workflows/release.yml) builds macOS
(arm64 and Intel), Windows and Linux, uploads the installers and any configured signed updates, then publishes
the release as **Latest**.

### Enable GitHub updates

Use your public FiveLeaf repository as the publisher. Run `go tool mygo keygen` and keep the generated
`mygo-update.key` somewhere secure outside the repository. Under the publisher's
**Settings → Secrets and variables → Actions**, add `MYGO_UPDATER_PUBLIC_KEY` under **Variables** with the contents
of `mygo-update.pub` and `MYGO_UPDATER_PRIVATE_KEY` under **Secrets** with the contents of `mygo-update.key`.
Keep the same key pair for future releases so installed copies continue to accept them. Once the latest
release has an update feed, each new release needs all six platform update manifests before it can become latest.

The workflow uses its own GitHub repository for the feed. An explicit `updates` block in `mygo.json` takes
precedence and also requires the matching private key secret. Install the first release built with the feed
configured. Subsequent updates appear in the upper-left corner and in **FiveLeaf → Check for Updates…**
(**Help** on Linux and Windows). The update window shows the release's changelog section.
In-app installation requires a supported, writable installation. Use the system package manager for a Linux system package.

### Require a minimum version

For the publisher's standard `v<version>` GitHub releases, the build embeds the URL of
[`update-policy.json`](update-policy.json) on the publisher's default branch. Publish the new version first,
then set `minimumVersion` in that file to the lowest supported stable version, such as `0.2.0`.
An older installation requires an update when it detects an installable release at or above that minimum.
Lower the value to relax the requirement or set it to `""` to disable it. The change takes effect on the next
successful update check. If you rename the default branch, keep the policy available at the original branch
for builds that still use it.

### Platform signing

The workflow supports the following repository secrets for macOS signing and notarization.

| Secret | What |
|---|---|
| `MACOS_CERTIFICATE` | the Developer ID Application certificate and its key, as a base64 `.p12` |
| `MACOS_CERTIFICATE_PASSWORD` | the password of the `.p12` |
| `APPLE_ID`, `APPLE_TEAM_ID` | the Apple ID of the developer account, and its team ID |
| `APPLE_APP_PASSWORD` | an app-specific password of the Apple ID, from account.apple.com |

macOS releases are signed with a Developer ID and notarized when the workflow has the complete certificate and
Apple credentials. Without them, releases and local builds use ad hoc signing. Windows builds are not signed.

The workflow embeds its own repository URL for problem reports using
`MYGO_LDFLAGS="-X main.repoURL=https://github.com/OWNER/REPO"`. Local builds without this URL copy a report
to the clipboard. Crash uploads are disabled unless `FIVELEAF_MON_URL` explicitly names a service.

After the first release, the workflow generates `Casks/fiveleaf.rb` from `scripts/fiveleaf.rb.in` with
that repository's disk images and their SHA-256 checksums. Until then, build from source.
To generate a cask locally, set `FIVELEAF_REPOSITORY=OWNER/REPO` and run
`sh scripts/update-homebrew.sh <version>`, then commit the generated file. The repository must allow
GitHub Actions to push to `main` for automatic cask updates. Pull `main` before releasing again.

Keep `LICENSE` intact. `mygo.json` includes it as a resource so each packaged app contains the original
copyright and MIT permission notice.
