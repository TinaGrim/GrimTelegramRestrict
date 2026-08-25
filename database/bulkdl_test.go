package database

import (
	"reflect"
	"testing"
)

func TestParseFailedIDs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []int
	}{
		{name: "empty", input: "", want: nil},
		{name: "whitespace only", input: "   ", want: nil},
		{name: "single id", input: "42", want: []int{42}},
		{name: "multiple ids", input: "1,2,3", want: []int{1, 2, 3}},
		{name: "spaces around ids", input: " 1 , 2 ", want: []int{1, 2}},
		{name: "invalid entries skipped", input: "1,abc,3", want: []int{1, 3}},
		{name: "all invalid", input: "x,y", want: []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFailedIDs(tt.input)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseFailedIDs(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestJoinIntIDsRoundTrip(t *testing.T) {
	ids := []int{7, 8, 9}
	parsed := ParseFailedIDs(joinIntIDs(ids))
	if !reflect.DeepEqual(parsed, ids) {
		t.Fatalf("round trip = %v, want %v", parsed, ids)
	}
	if joinIntIDs(nil) != "" {
		t.Fatalf("joinIntIDs(nil) should be empty")
	}
}
