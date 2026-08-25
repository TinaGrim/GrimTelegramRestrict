package database

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

func GetBulkDLProgress(ctx context.Context, userID uint, chatID int64) (*BulkDLProgress, error) {
	var progress BulkDLProgress
	err := db.WithContext(ctx).Where("user_id = ? AND chat_id = ?", userID, chatID).First(&progress).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &progress, nil
}

// SaveBulkDLProgress creates or updates the resume checkpoint for the given
// user and chat. failedIDs is persisted as-is (may be empty).
func SaveBulkDLProgress(ctx context.Context, userID uint, chatID int64, lastReadMessageID int, failedIDs []int) error {
	progress, err := GetBulkDLProgress(ctx, userID, chatID)
	if err != nil {
		return err
	}
	if progress == nil {
		progress = &BulkDLProgress{
			UserID: userID,
			ChatID: chatID,
		}
	}
	progress.LastReadMessageID = lastReadMessageID
	progress.FailedIDs = joinIntIDs(failedIDs)
	return db.WithContext(ctx).Save(progress).Error
}

func DeleteBulkDLProgress(ctx context.Context, userID uint, chatID int64) error {
	return db.WithContext(ctx).
		Where("user_id = ? AND chat_id = ?", userID, chatID).
		Unscoped().Delete(&BulkDLProgress{}).Error
}

func ParseFailedIDs(raw string) []int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func joinIntIDs(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.Itoa(id))
	}
	return strings.Join(parts, ",")
}
