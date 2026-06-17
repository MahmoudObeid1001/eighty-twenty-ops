package handlers

import "testing"

func TestLooksEnglishGradeNote(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "english note",
			text: "He has only attended for around two times and then disappeared.",
			want: true,
		},
		{
			name: "arabic note",
			text: "هو حضر مرتين فقط ثم اختفى بعد ذلك.",
			want: false,
		},
		{
			name: "mixed note with arabic",
			text: "He attended مرتين فقط and then disappeared.",
			want: false,
		},
		{
			name: "short fragment",
			text: "good work",
			want: true,
		},
		{
			name: "empty",
			text: "   ",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksEnglishGradeNote(tc.text); got != tc.want {
				t.Fatalf("looksEnglishGradeNote(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}
