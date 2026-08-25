---
title: "Bulk Channel Download"
weight: 5
---

# Bulk Channel Download

{{< hint warning >}}
This feature requires enabling UserBot integration.
{{< /hint >}}

Download all media from a channel's history to a storage of your choice. This is the Telegram-side equivalent of [telegram_media_downloader](https://github.com/Dineshkarthik/telegram_media_downloader): you give it a channel ID, and it walks the channel history, downloading every matching media file.

```
/bulkdl <channel_id_or_username> [media_types] [max_files]
```

- `<channel_id_or_username>`: numeric chat ID (bot-API style `-100...` works) or `@username`
- `[media_types]`: comma-separated list from `photo`, `video`, `video_note`, `audio`, `voice`, `document`; omit it or use `all` to include every type
- `[max_files]`: stop after this many files in one run (optional). A bare number is treated as `max_files` and includes **all** types

Examples:

```
/bulkdl @some_channel                # all types
/bulkdl -1001234567890 100           # all types, max 100 files
/bulkdl -1001234567890 photo,video 100
```

The bot will ask you to select a storage (and directory if configured), then start downloading. Progress is edited into the message with a cancel button.

## Step-by-step wizard

Run `/bulkdl <channel_id_or_username>` without any other arguments and the bot guides you through inline buttons:

1. **Media types** — `all`, or a single type (`photo`, `video`, ...)
2. **Max files** — `∞` (no limit), 50, 100, 500, 1000
3. **Storage** — as with any other save

Power users can skip the wizard by passing the arguments directly, e.g. `/bulkdl -1001234567890 photo,video 100`.

## Resuming

Progress is checkpointed per channel after every batch of messages. If the task is interrupted — crash, network failure, or cancellation — run `/bulkdl` again for the same channel and it resumes from where it stopped instead of re-downloading everything.

Messages whose download failed are remembered and retried automatically on the next run. To discard the saved progress and start over, use:

```
/bulkdl reset <channel_id_or_username>
```

## Credits

- Bulk channel download was ported into SaveAny-Bot from [Dineshkarthik/telegram_media_downloader](https://github.com/Dineshkarthik/telegram_media_downloader) by [Dineshkarthik](https://github.com/Dineshkarthik).
- SaveAny-Bot is created and maintained by [krau](https://github.com/krau).
