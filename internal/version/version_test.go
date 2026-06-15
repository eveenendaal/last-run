package version

import "testing"

func TestGetNextVersion(t *testing.T) {
	cases := []struct {
		tag  string
		want string
	}{
		{"", "1.0.0"},
		{"v1.0.0", "1.1.0"},
		{"v1.5.0", "1.6.0"},
		{"v2.3.0", "2.4.0"},
	}
	for _, c := range cases {
		if got := GetNextVersion(c.tag); got != c.want {
			t.Errorf("GetNextVersion(%q) = %q, want %q", c.tag, got, c.want)
		}
	}
}
