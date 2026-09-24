# 第三方依赖与许可证记录

生成依据：仓库锁定的 `go.mod`、`go list -deps ./cmd/kino`、`package-lock.json`和模块许可证文件。它是发布审计输入，不替代各依赖自带的标准许可证全文。

## 分发目标 Go 二进制的运行依赖并集

| 模块 | 版本 | 主许可证 |
|---|---:|---|
| `github.com/dustin/go-humanize` | v1.0.1 | MIT |
| `github.com/google/uuid` | v1.6.0 | BSD-3-Clause |
| `github.com/mattn/go-isatty` | v0.0.24 | MIT |
| `github.com/ncruces/go-strftime` | v1.0.0 | MIT |
| `github.com/remyoudompheng/bigfft` | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause |
| `golang.org/x/sys` | v0.47.0 | BSD-3-Clause |
| `modernc.org/libc` | v1.75.6 | BSD-3-Clause；另含上游第三方声明 |
| `modernc.org/mathutil` | v1.7.1 | BSD-3-Clause；子组件另有声明 |
| `modernc.org/memory` | v1.12.1 | BSD-3-Clause；mmap-go/Go 来源另有声明 |
| `modernc.org/sqlite` | v1.58.0 | BSD-3-Clause；另含 SQLite public-domain 声明与 sqlite-vec MIT 许可 |

本表是 Linux amd64/arm64/armv7、Windows amd64/arm64 与 macOS arm64 无 CGO 发布目标的依赖并集；某个具体目标可以只链接其中的子集。模块版本和校验和由 `go.sum` 锁定。发布自动化不得从本表推断“只有一种许可证”；必须读取对应模块的 `LICENSE` 以及 `LICENSE-3RD-PARTY.md`、`LICENSE-*` 等附加文件。


## Web E2E（开发/测试，不进入服务端运行镜像）

| 包 | 版本 | 许可证 |
|---|---:|---|
| `@playwright/test` / `playwright` / `playwright-core` | 1.62.1 | Apache-2.0 |
| `fsevents` | 2.3.2 | MIT；仅 macOS 可选开发依赖 |

## 发布检查

- `go mod verify`
- `go list -deps -f '{{with .Module}}{{.Path}} {{.Version}}{{end}}' ./cmd/kino | sort -u`
- `./scripts/check-third-party-notices.sh --all`
- `./scripts/collect-third-party-licenses.sh NEW_EMPTY_DIRECTORY`
- `go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...`
- `npm audit`
- 对最终 Go 二进制执行 `go version -m`。
- 将项目 `LICENSE`、需要的 `NOTICE`、本文件及 `collect-third-party-licenses.sh` 从当前模块缓存收集的实际第三方许可证正文一起放入 release 资产和桌面归档。

Kino 自身采用 Apache-2.0，标准全文位于仓库根目录 `LICENSE`；本文件只记录第三方组件及其各自适用的许可证与附加声明。
