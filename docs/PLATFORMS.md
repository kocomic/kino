# 平台预设

## 设计目标

平台注册表解决“同一个平台在目录、前端和用户口语中名称不同”的问题。例如：

```text
红白机 / Famicom / FC / NES  →  规范 ID: nes
Mega Drive / Genesis / MD     →  规范 ID: megadrive
PlayStation / PS1 / PSX       →  规范 ID: psx
Nintendo 3DS / N3DS           →  规范 ID: 3ds
```

## 内置范围

当前注册表内置 72 个常用平台，范围以 Switch 世代为上限，覆盖：

- Nintendo：NES、Famicom Disk System、SNES、N64、64DD、GameCube、Wii/WiiWare、Wii U、Switch、GB、GBC、GBA、NDS、3DS、Virtual Boy、Game & Watch、Pokémon Mini。
- Sony：PlayStation、PS2、PS3、PSP、PS Vita。
- Microsoft：Xbox、Xbox 360。
- Sega 与街机：SG-1000、Master System、Mega Drive/Genesis、Mega-CD/Sega CD、32X、Game Gear、Saturn、Dreamcast、NAOMI、Atomiswave 和通用街机。
- 经典主机与掌机：Atari、NEC、SNK、Bandai、3DO、ColecoVision、Intellivision、Vectrex、CD-i 和 PICO-8。
- 经典电脑：Apple II、Amiga、Commodore 64、Amstrad CPC、ZX Spectrum、MSX/MSX2、Atari 8-bit/ST、PC-88、PC-98、X68000、FM Towns、DOS 和 ScummVM。

PS4、PS5、Xbox One 和 Xbox Series 不在内置范围内，但仍可用自定义平台表达。这里的“最高到 Switch”是默认目录边界，不是阻止用户管理更晚平台的数据库限制。

## 资料库中的平台

平台记录包含规范 ID、显示名、别名、ROM 扩展名和 ES-DE 目录映射。
扫描器会归一化平台别名，并使用平台对应的文件格式检查候选 ROM。
源数据位于 [platforms.json](../internal/platforms/platforms.json)，
加载和别名解析位于 [registry.go](../internal/platforms/registry.go)。

```http
GET /api/v1/platforms
GET /api/v1/platforms/ps1
GET /api/v1/custom-platforms
POST /api/v1/custom-platforms
PUT /api/v1/custom-platforms/{id}
DELETE /api/v1/custom-platforms/{id}
```

`ps1` 会通过别名解析为 `psx`。命令行可使用 `kino platforms`，或通过
`kino platforms --db ./data/library.db` 同时列出已启用的自定义平台。

## 自定义平台

自定义平台保存在 SQLite 中。规范 ID 不可变；显示名、别名、扩展名、
目录型格式、ES-DE 映射和启用状态可配置。停用不会删除定义或已有游戏。
被资料库对象引用的平台不能直接删除。启用时，所有 ID、别名和 ES-DE 名称
必须与现有注册表无冲突。

`library-manifest.json` v6 只携带条目实际引用的自定义平台。预览验证定义和
标识冲突；提交将缺失定义与所选游戏、版本放入同一事务。已有定义须字段一致
且处于启用状态，导入不会覆盖或重新启用本地设置。`db-check` 检查字段约束
及组合注册表的一致性。
