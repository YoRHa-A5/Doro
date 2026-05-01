package emoji

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		wantName []string
		wantID   []string
	}{
		{
			name:     "static emoji",
			input:    "hello <:cool:123456789>",
			wantLen:  1,
			wantName: []string{"cool"},
			wantID:   []string{"123456789"},
		},
		{
			name:     "animated emoji",
			input:    "hello <a:fire:987654321>",
			wantLen:  1,
			wantName: []string{"fire"},
			wantID:   []string{"987654321"},
		},
		{
			name:     "multiple different emojis",
			input:    "hello <:a:1> and <:b:2>",
			wantLen:  2,
			wantName: []string{"a", "b"},
			wantID:   []string{"1", "2"},
		},
		{
			name:    "same emoji twice",
			input:   "hello <:cool:123> and <:cool:123>",
			wantLen: 2,
			wantName: []string{"cool", "cool"},
			wantID:   []string{"123", "123"},
		},
		{
			name:    "no emojis",
			input:   "just text",
			wantLen: 0,
		},
		{
			name:    "empty string",
			input:   "",
			wantLen: 0,
		},
		{
			name:    "emoji with spaces around",
			input:   "  <:name:123>  ",
			wantLen: 1,
			wantName: []string{"name"},
			wantID:   []string{"123"},
		},
		{
			name:    "emoji at start",
			input:   "<:start:1> hello world",
			wantLen: 1,
			wantName: []string{"start"},
			wantID:   []string{"1"},
		},
		{
			name:    "emoji at end",
			input:   "hello world <:end:2>",
			wantLen: 1,
			wantName: []string{"end"},
			wantID:   []string{"2"},
		},
		{
			name:    "mixed text and emojis",
			input:   "hello <:a:1> world <:b:2> test",
			wantLen: 2,
			wantName: []string{"a", "b"},
			wantID:   []string{"1", "2"},
		},
		{
			name:    "unicode text",
			input:   "hello 😀 world",
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if len(got) != tt.wantLen {
				t.Errorf("got %d emojis, want %d", len(got), tt.wantLen)
				return
			}
			for i := range tt.wantName {
				if got[i].Name != tt.wantName[i] {
					t.Errorf("emoji %d name: got %s, want %s", i, got[i].Name, tt.wantName[i])
				}
				if got[i].ID != tt.wantID[i] {
					t.Errorf("emoji %d ID: got %s, want %s", i, got[i].ID, tt.wantID[i])
				}
			}
		})
	}
}
