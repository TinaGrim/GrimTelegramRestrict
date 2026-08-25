---
title: "频道批量下载"
weight: 5
---

# 频道批量下载

{{< hint warning >}}
该功能需开启 UserBot 集成.
{{< /hint >}}

将频道历史记录中的所有媒体批量下载到指定的存储端. 类似 [telegram_media_downloader](https://github.com/Dineshkarthik/telegram_media_downloader): 输入频道 ID, 机器人会遍历频道历史, 下载所有匹配的媒体文件.

```
/bulkdl <频道ID或用户名> [媒体类型] [最大文件数]
```

- `<频道ID或用户名>`: 数字 ID (支持 `-100` 开头的 Bot API 风格 ID) 或 `@用户名`
- `[媒体类型]`: `photo`, `video`, `video_note`, `audio`, `voice`, `document` 的逗号分隔列表; 省略或使用 `all` 表示包含所有类型
- `[最大文件数]`: 单次运行最多下载的文件数 (可选). 直接给数字时视为最大文件数并包含**所有**类型

示例:

```
/bulkdl @some_channel                # 所有类型
/bulkdl -1001234567890 100           # 所有类型, 最多 100 个文件
/bulkdl -1001234567890 photo,video 100
```

机器人会让你选择存储位置 (以及已配置的目录), 然后开始下载. 进度会实时更新在消息中, 并带有取消按钮.

## 分步向导

直接运行 `/bulkdl <频道ID或用户名>` (不带其他参数), 机器人会通过内联按钮一步步引导:

1. **媒体类型** — `all`, 或单一类型 (`photo`, `video`, ...)
2. **最大文件数** — `∞` (不限制), 50, 100, 500, 1000
3. **存储位置** — 与其他保存操作一致

熟悉参数的用户也可以跳过向导直接传参, 例如 `/bulkdl -1001234567890 photo,video 100`.

## 断点续传

进度按频道逐批保存. 如果任务中断 — 崩溃、网络故障或手动取消 — 对同一频道再次运行 `/bulkdl` 即可从上次停止处继续, 而无需重新下载.

下载失败的消息会被记录, 并在下一次运行时自动重试. 如需丢弃已保存的进度从头开始:

```
/bulkdl reset <频道ID或用户名>
```

## 致谢

- 频道批量下载功能移植自 [Dineshkarthik/telegram_media_downloader](https://github.com/Dineshkarthik/telegram_media_downloader), 作者为 [Dineshkarthik](https://github.com/Dineshkarthik).
- SaveAny-Bot 由 [krau](https://github.com/krau) 创建并维护.
