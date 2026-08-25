package bulkdl

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gotd/td/tg"
)

// MediaType is a media kind eligible for bulk download. The set and the
// classification rules mirror telegram_media_downloader's get_media_type.
//
// Bulk download support was ported into SaveAny-Bot from
// github.com/Dineshkarthik/telegram_media_downloader by Dineshkarthik;
// SaveAny-Bot itself is maintained by krau (github.com/krau).
type MediaType string

const (
	MediaTypePhoto     MediaType = "photo"
	MediaTypeVideo     MediaType = "video"
	MediaTypeAudio     MediaType = "audio"
	MediaTypeVoice     MediaType = "voice"
	MediaTypeVideoNote MediaType = "video_note"
	MediaTypeDocument  MediaType = "document"
)

// AllMediaTypes returns every supported media type.
func AllMediaTypes() []MediaType {
	return []MediaType{
		MediaTypePhoto,
		MediaTypeVideo,
		MediaTypeVideoNote,
		MediaTypeAudio,
		MediaTypeVoice,
		MediaTypeDocument,
	}
}

// ParseCommandArgs parses the optional trailing arguments of /bulkdl (the
// channel argument is handled by the caller). Accepted forms:
//
//	(none)                -> all types, no cap
//	"all"                 -> all types, no cap
//	"photo,video"         -> given types, no cap
//	"100"                 -> all types, capped at 100
//	"photo,video" "100"   -> given types, capped at 100
//
// It returns the raw types input ("", "all" or a comma list) and the max
// file count; use ParseMediaTypes on the returned types input.
func ParseCommandArgs(args []string) (typesInput string, maxMessages int, err error) {
	if len(args) == 0 {
		return "", 0, nil
	}
	first := strings.TrimSpace(args[0])
	if n, numErr := strconv.Atoi(first); numErr == nil && n >= 0 {
		typesInput = ""
		maxMessages = n
		if len(args) > 1 {
			return "", 0, fmt.Errorf("unexpected extra argument: %s", args[1])
		}
		return typesInput, maxMessages, nil
	}
	typesInput = first
	if len(args) > 1 {
		n, numErr := strconv.Atoi(strings.TrimSpace(args[1]))
		if numErr != nil || n < 0 {
			return "", 0, fmt.Errorf("invalid max files value: %s", args[1])
		}
		maxMessages = n
	}
	return typesInput, maxMessages, nil
}

// ParseMediaTypes parses a user-supplied comma-separated type list.
// An empty list or "all" selects every supported type.
func ParseMediaTypes(input string) ([]MediaType, error) {
	input = strings.TrimSpace(input)
	if input == "" || strings.EqualFold(input, "all") {
		return AllMediaTypes(), nil
	}
	parts := strings.Split(input, ",")
	types := make([]MediaType, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if part == "all" {
			return AllMediaTypes(), nil
		}
		mt := MediaType(part)
		if !isValidMediaType(mt) {
			return nil, fmt.Errorf("unsupported media type: %s", part)
		}
		if !containsMediaType(types, mt) {
			types = append(types, mt)
		}
	}
	if len(types) == 0 {
		return AllMediaTypes(), nil
	}
	return types, nil
}

func isValidMediaType(mt MediaType) bool {
	for _, known := range AllMediaTypes() {
		if mt == known {
			return true
		}
	}
	return false
}

func containsMediaType(types []MediaType, mt MediaType) bool {
	for _, t := range types {
		if t == mt {
			return true
		}
	}
	return false
}

type MediaTypeSet map[MediaType]struct{}

func NewMediaTypeSet(types []MediaType) MediaTypeSet {
	set := make(MediaTypeSet, len(types))
	for _, t := range types {
		set[t] = struct{}{}
	}
	return set
}

func (s MediaTypeSet) Contains(mt MediaType) bool {
	_, ok := s[mt]
	return ok
}

// ClassifyMessageMedia maps a Telegram message media to a MediaType.
// Ported from telegram_media_downloader's get_media_type; returns false for
// unsupported media (web previews, paid media, polls, etc).
func ClassifyMessageMedia(media tg.MessageMediaClass) (MediaType, bool) {
	switch m := media.(type) {
	case *tg.MessageMediaPhoto:
		return MediaTypePhoto, true
	case *tg.MessageMediaDocument:
		doc, ok := m.Document.AsNotEmpty()
		if !ok {
			return "", false
		}
		for _, attr := range doc.Attributes {
			switch a := attr.(type) {
			case *tg.DocumentAttributeAudio:
				if a.Voice {
					return MediaTypeVoice, true
				}
				return MediaTypeAudio, true
			case *tg.DocumentAttributeVideo:
				if a.RoundMessage {
					return MediaTypeVideoNote, true
				}
				return MediaTypeVideo, true
			}
		}
		return MediaTypeDocument, true
	default:
		return "", false
	}
}
