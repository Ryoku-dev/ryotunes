package look

import "testing"

func TestClassForPlayerBreakpoints(t *testing.T) {
	tests := []struct {
		width float32
		want  Class
	}{
		{width: 759.9, want: Compact},
		{width: 760, want: Medium},
		{width: 1099.9, want: Medium},
		{width: 1100, want: Expanded},
	}
	for _, test := range tests {
		if got := ClassFor(test.width); got != test.want {
			t.Errorf("ClassFor(%v) = %v, want %v", test.width, got, test.want)
		}
	}
}
