package database

import (
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	ChatID           int64 `gorm:"uniqueIndex;not null"`
	Silent           bool
	DefaultStorage   string
	DefaultDir       uint // Dir.ID
	Dirs             []Dir
	ApplyRule        bool
	Rules            []Rule
	WatchChats       []WatchChat
	FilenameStrategy string
	FilenameTemplate string
	ConflictStrategy string
}

type WatchChat struct {
	gorm.Model
	UserID uint // User's database ID (not chat ID)
	ChatID int64
	Filter string
}

type Dir struct {
	gorm.Model
	UserID      uint
	StorageName string
	Path        string
}

type Rule struct {
	gorm.Model
	UserID      uint
	Type        string
	Data        string
	StorageName string
	DirPath     string
}

// BulkDLProgress stores the resume state of a bulk channel media download
// (mirrors telegram_media_downloader's per-chat last_read_message_id).
type BulkDLProgress struct {
	gorm.Model
	UserID uint  `gorm:"uniqueIndex:idx_bulkdl_user_chat"`
	ChatID int64 `gorm:"uniqueIndex:idx_bulkdl_user_chat"`
	// LastReadMessageID is the ID of the last message scanned for this chat;
	// the next run resumes from messages with greater IDs.
	LastReadMessageID int
	// FailedIDs holds comma-separated message IDs whose downloads failed and
	// will be retried on the next run.
	FailedIDs string
}
