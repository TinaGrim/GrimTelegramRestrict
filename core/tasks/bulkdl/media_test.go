package bulkdl

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestParseMediaTypes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []MediaType
		wantErr bool
	}{
		{name: "empty defaults to all", input: "", want: AllMediaTypes()},
		{name: "all keyword", input: "all", want: AllMediaTypes()},
		{name: "all keyword case insensitive", input: "ALL", want: AllMediaTypes()},
		{
			name:  "single type",
			input: "photo",
			want:  []MediaType{MediaTypePhoto},
		},
		{
			name:  "multiple types with spaces",
			input: "photo, video , voice",
			want:  []MediaType{MediaTypePhoto, MediaTypeVideo, MediaTypeVoice},
		},
		{
			name:  "duplicates removed",
			input: "video,video,document",
			want:  []MediaType{MediaTypeVideo, MediaTypeDocument},
		},
		{
			name:  "all mixed with types",
			input: "photo,all,video",
			want:  AllMediaTypes(),
		},
		{
			name:    "invalid type",
			input:   "photo,sticker",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMediaTypes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseMediaTypes(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ParseMediaTypes(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ParseMediaTypes(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestClassifyMessageMedia(t *testing.T) {
	newDoc := func(attrs ...tg.DocumentAttributeClass) *tg.MessageMediaDocument {
		return &tg.MessageMediaDocument{
			Document: &tg.Document{
				ID:         1,
				Attributes: attrs,
				MimeType:   "application/octet-stream",
			},
		}
	}

	tests := []struct {
		name     string
		media    tg.MessageMediaClass
		wantType MediaType
		wantOK   bool
	}{
		{
			name:     "photo",
			media:    &tg.MessageMediaPhoto{Photo: &tg.Photo{ID: 1}},
			wantType: MediaTypePhoto,
			wantOK:   true,
		},
		{
			name:     "plain document",
			media:    newDoc(&tg.DocumentAttributeFilename{FileName: "a.zip"}),
			wantType: MediaTypeDocument,
			wantOK:   true,
		},
		{
			name:     "audio",
			media:    newDoc(&tg.DocumentAttributeAudio{Voice: false, Title: "song"}),
			wantType: MediaTypeAudio,
			wantOK:   true,
		},
		{
			name:     "voice note",
			media:    newDoc(&tg.DocumentAttributeAudio{Voice: true}),
			wantType: MediaTypeVoice,
			wantOK:   true,
		},
		{
			name:     "video",
			media:    newDoc(&tg.DocumentAttributeVideo{Duration: 10}, &tg.DocumentAttributeFilename{FileName: "clip.mp4"}),
			wantType: MediaTypeVideo,
			wantOK:   true,
		},
		{
			name:     "round video message",
			media:    newDoc(&tg.DocumentAttributeVideo{Duration: 3, RoundMessage: true}),
			wantType: MediaTypeVideoNote,
			wantOK:   true,
		},
		{
			name:     "empty document unsupported",
			media:    &tg.MessageMediaDocument{Document: &tg.DocumentEmpty{}},
			wantType: "",
			wantOK:   false,
		},
		{
			name:     "webpage unsupported",
			media:    &tg.MessageMediaWebPage{Webpage: &tg.WebPageEmpty{}},
			wantType: "",
			wantOK:   false,
		},
		{
			name:     "nil-ish contact unsupported",
			media:    &tg.MessageMediaContact{},
			wantType: "",
			wantOK:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotOK := ClassifyMessageMedia(tt.media)
			if gotOK != tt.wantOK || gotType != tt.wantType {
				t.Fatalf("ClassifyMessageMedia() = (%q, %v), want (%q, %v)", gotType, gotOK, tt.wantType, tt.wantOK)
			}
		})
	}
}

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantTypes string
		wantMax   int
		wantErr   bool
	}{
		{name: "no args", args: nil, wantTypes: "", wantMax: 0},
		{name: "all keyword", args: []string{"all"}, wantTypes: "all", wantMax: 0},
		{name: "types only", args: []string{"photo,video"}, wantTypes: "photo,video", wantMax: 0},
		{name: "bare number means max with all types", args: []string{"100"}, wantTypes: "", wantMax: 100},
		{name: "types and max", args: []string{"photo,video", "100"}, wantTypes: "photo,video", wantMax: 100},
		{
			name:    "extra arg after bare number",
			args:    []string{"100", "200"},
			wantErr: true,
		},
		{
			name:    "invalid max after types",
			args:    []string{"photo", "abc"},
			wantErr: true,
		},
		{
			name:    "negative max",
			args:    []string{"-5"},
			wantErr: false,
			// "-5" is not treated as a number for the types slot; it fails
			// type validation later via ParseMediaTypes
			wantTypes: "-5",
			wantMax:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTypes, gotMax, err := ParseCommandArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseCommandArgs(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if gotTypes != tt.wantTypes || gotMax != tt.wantMax {
				t.Fatalf("ParseCommandArgs(%v) = (%q, %d), want (%q, %d)",
					tt.args, gotTypes, gotMax, tt.wantTypes, tt.wantMax)
			}
		})
	}
}

func TestMediaTypeSet(t *testing.T) {
	set := NewMediaTypeSet([]MediaType{MediaTypePhoto, MediaTypeVideo})
	if !set.Contains(MediaTypePhoto) || !set.Contains(MediaTypeVideo) {
		t.Fatal("expected set to contain photo and video")
	}
	if set.Contains(MediaTypeAudio) {
		t.Fatal("expected set not to contain audio")
	}
}
