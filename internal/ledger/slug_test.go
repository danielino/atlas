package ledger

import "testing"

func TestSlugify(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  string
	}{
		{"simple", "Fix container reconcile retry", "fix-container-reconcile-retry"},
		{"punctuation stripped", "Fix: retry (again)!", "fix-retry-again"},
		{"multiple spaces collapse", "too    many   spaces", "too-many-spaces"},
		{"already lowercase with dashes", "already-lower-case", "already-lower-case"},
		{"numbers kept", "Upgrade to v2 API", "upgrade-to-v2-api"},
		{"leading/trailing punctuation trimmed", "!!!hello!!!", "hello"},
		{"empty title", "", ""},
		{"only punctuation", "???", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Slugify(tc.title)
			if got != tc.want {
				t.Errorf("Slugify(%q) = %q, want %q", tc.title, got, tc.want)
			}
		})
	}
}

func TestSlugify_MaxLength40(t *testing.T) {
	title := "this is a very long title that definitely exceeds forty characters in length"
	got := Slugify(title)
	if len(got) > 40 {
		t.Errorf("Slugify() returned slug longer than 40 chars: %q (%d)", got, len(got))
	}
}

func TestSlugify_MaxLengthDoesNotEndInDash(t *testing.T) {
	// Construct a title whose 40-char truncation would land exactly on a
	// separator; ensure trailing dashes are trimmed.
	title := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa bbbbbbbbbb"
	got := Slugify(title)
	if len(got) > 0 && got[len(got)-1] == '-' {
		t.Errorf("Slugify() left a trailing dash: %q", got)
	}
}
