package eval

import "testing"

func TestParseVerdict(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    float64
		wantErr bool
	}{
		{"plain", `{"score": 0.9, "reason": "good"}`, 0.9, false},
		{"fenced", "```json\n{\"score\": 0.5, \"reason\": \"meh\"}\n```", 0.5, false},
		{"prose around", "Here is my verdict:\n{\"score\": 0.75, \"reason\": \"ok\"}\nThanks", 0.75, false},
		{"no json", "I cannot grade this.", 0, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			v, err := parseVerdict(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", v)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v.Score != tt.want {
				t.Errorf("Score = %v, want %v", v.Score, tt.want)
			}
		})
	}
}
