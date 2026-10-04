package process_event

import "testing"

func TestJoinNotice(t *testing.T) {
	tests := []struct {
		name     string
		notice   string
		warnings []string
		want     string
	}{
		{"nothing", "", nil, ""},
		{"notice only", "n", nil, "n"},
		{"warnings only", "", []string{"a", "b"}, "a\n\nb"},
		{"both", "n", []string{"a"}, "n\n\na"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinNotice(tt.notice, tt.warnings); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
