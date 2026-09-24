# 部署 Kino

镜像：`ghcr.io/kocomic/kino:edge`，镜像副本：`docker.io/kocomic/kino:edge`。
支持 Linux amd64 / arm64。正式使用建议固定镜像摘要；edge 标签随通过验证的构建更新。

1. 创建 ROM 目录及私有数据目录，例如 `/volume1/games` 和 `/volume1/docker/kino/data`。
2. 数据目录须位于本地文件系统，不能将 SQLite 放到 SMB/NFS。让 UID/GID 10001
   能写入数据目录、读取 ROM 目录。
3. 复制 `.env.ghcr.example` 为 `.env`，填写实际目录并设置随机
   `GAME_LIBRARY_TOKEN`（可用 `openssl rand -hex 32` 生成）。
4. 使用 `compose.ghcr.yaml` 创建项目：

```sh
docker compose -f compose.ghcr.yaml pull
docker compose -f compose.ghcr.yaml up -d
```

在浏览器访问 `http://NAS地址:8080`，到设置输入令牌。
数据库、媒体、受管 ROM 与恢复数据保存在 `/data`；原始 ROM 只读挂载到 `/library`。

## 更新现有实例

更新前保存完整状态备份和当前镜像摘要。保持数据路径不变，执行上面的 pull 和 up
命令。固定摘要的实例需先更新镜像配置。更新后检查资料库数量、封面和导入预览。

## 备份和恢复

参阅 `kino backup-state --help`、`kino check-state --help`、
`kino restore-state --help` 及 `scripts/nas-backup.sh`。
ROM 引用文件应另行备份。恢复使用新的目录，校验后再切换。

本地构建使用 `./scripts/build-local.sh`，容器本地构建使用 `compose.yaml`。
