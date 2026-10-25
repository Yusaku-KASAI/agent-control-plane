package slackbot

import (
	"encoding/json"
	"os"
	"testing"
)

// The cases are shared with the slack plugin's check-markdown, which is tried on the same text:
// it finds no bare URL and no table right under text in what slackMarkdown gives.
const markdownCases = "../../../platform/plugins/slack/tests/markdown-cases.json"

func TestSlackMarkdown(t *testing.T) {
	b, err := os.ReadFile(markdownCases)
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name  string  `json:"name"`
		Text  string  `json:"text"`
		Fixed *string `json:"fixed"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			want := tt.Text
			if tt.Fixed != nil {
				want = *tt.Fixed
			}
			if got := slackMarkdown(tt.Text); got != want {
				t.Errorf("slackMarkdown(%q)\n got %q\nwant %q", tt.Text, got, want)
			}
		})
	}
}
