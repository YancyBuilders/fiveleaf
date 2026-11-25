<a name="contributing"></a>

# 贡献指南

简体中文 · [English](CONTRIBUTING.md)

<a name="development"></a>

## 开发

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

`scripts/screenshots/shoot.sh` 为各功能生成浅色和深色截图，存入 `build/screenshots/raw`，再为截图加上渐变背景并保存到 `build/screenshots/framed`。README 使用的 1600 像素宽截图保存在 `docs/screenshots`。运行环境需要 macOS、安装了 Pillow 的 Python 和 `pngquant`。如需单独截取某项功能的画面，运行 `FSHOTS=<dir> go test -run TestFeatColumns ./cmd/fiveleaf`。

<a name="releasing"></a>

## 发布

按 [AGENTS.md](AGENTS.md) 的说明，将发布说明写入 `CHANGELOG.md` 的 `## Unreleased` 小节并提交。然后运行以下命令。

```sh
scripts/bump.sh patch    # or minor, major, or a version such as 0.2.0
```

脚本会更新 `mygo.json` 中的版本号，将 `CHANGELOG.md` 的 `## Unreleased` 改名为 `## <version>`，提交改动，再推送 `main` 和 `v<version>` 标签。[发布工作流](.github/workflows/release.yml) 会为 macOS（arm64 和 Intel）、Windows 和 Linux 构建安装包，上传安装包和已配置的签名更新，然后将该版本发布为 **Latest**。

<a name="enable-github-updates"></a>

### 启用 GitHub 更新

使用自己的公开 FiveLeaf 仓库作为发布方。运行 `go tool mygo keygen`，将生成的 `mygo-update.key` 存放在仓库以外的安全位置。在发布仓库的 **Settings → Secrets and variables → Actions** 中，将 `mygo-update.pub` 的内容添加到 **Variables**，命名为 `MYGO_UPDATER_PUBLIC_KEY`，将 `mygo-update.key` 的内容添加到 **Secrets**，命名为 `MYGO_UPDATER_PRIVATE_KEY`。后续版本继续使用同一对密钥，已安装的应用才能验证更新。一旦最新版本启用了更新源，后续各版都需要包含六个平台的更新清单，才能成为最新版本。

工作流默认以自身所在的 GitHub 仓库作为更新源。如果在 `mygo.json` 里显式配置了 `updates`，则以该配置为准，同时需要提供匹配的私钥 Secret。先安装首个启用了更新源的版本。后续更新会出现在窗口左上角，也可以通过 **FiveLeaf → 检查更新…** 查看。在 Linux 和 Windows 上，该命令位于 **帮助** 菜单。更新窗口会展示该版本的更新日志。

应用内安装更新需要已安装版本支持此功能，且安装目录可写。通过 Linux 系统包安装的版本由系统包管理器更新。

<a name="require-a-minimum-version"></a>

### 设定最低版本

对于发布仓库中使用标准 `v<version>` 标签的 GitHub 发行版，构建时会嵌入该仓库默认分支上 [`update-policy.json`](update-policy.json) 的 URL。先发布新版本，再将该文件里的 `minimumVersion` 设为支持的最低稳定版，例如 `0.2.0`。较旧版本检测到有达到最低要求的版本可安装时，会要求用户更新。降低该值可放宽要求，设为 `""` 可关闭要求。变更会在下一次成功检查更新时生效。如果重命名默认分支，还需让原分支上的策略文件可供仍引用它的版本访问。

<a name="platform-signing"></a>

### 平台签名

工作流支持使用以下仓库 Secret 为 macOS 版本签名和公证。

| Secret | 用途 |
|---|---|
| `MACOS_CERTIFICATE` | Developer ID Application 证书及其密钥，以 Base64 编码的 `.p12` 提供 |
| `MACOS_CERTIFICATE_PASSWORD` | `.p12` 文件的密码 |
| `APPLE_ID`, `APPLE_TEAM_ID` | 开发者账号的 Apple ID 和团队 ID |
| `APPLE_APP_PASSWORD` | 在 account.apple.com 为 Apple ID 生成的 App 专用密码 |

工作流具备完整的证书和 Apple 凭据时，macOS 发行版会使用 Developer ID 签名并进行公证。没有这些凭据时，发行构建和本地构建都采用临时签名（ad hoc）。Windows 构建不进行签名。

工作流通过 `MYGO_LDFLAGS="-X main.repoURL=https://github.com/OWNER/REPO"` 嵌入自身仓库的 URL，以便用户报告问题。未嵌入该 URL 的本地构建会将报告复制到剪贴板。仅在 `FIVELEAF_MON_URL` 显式指定服务时才上传崩溃报告。

首次发布后，工作流会以 `scripts/fiveleaf.rb.in` 为模板，使用仓库发行版中的磁盘映像及其 SHA-256 校验值生成 `Casks/fiveleaf.rb`。首次发布前请从源码构建。如需在本地生成 Cask，设置 `FIVELEAF_REPOSITORY=OWNER/REPO`，运行 `sh scripts/update-homebrew.sh <version>`，然后提交生成的文件。自动更新 Cask 需要仓库允许 GitHub Actions 推送到 `main`。下次发布前，先拉取 `main`。

请完整保留 `LICENSE`。`mygo.json` 会将它作为资源打包，让每份应用都包含原有版权声明和 MIT 许可条款。
