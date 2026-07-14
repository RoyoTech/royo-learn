package selfupdate

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "patch newer", a: "v0.1.9", b: "v0.1.8", want: 1},
		{name: "equal with and without v prefix", a: "0.1.8", b: "v0.1.8", want: 0},
		{name: "patch older", a: "v0.1.7", b: "0.1.8", want: -1},
		{name: "major beats minor and patch", a: "v1.0.0", b: "v0.9.9", want: 1},
		{name: "numeric not lexicographic", a: "v0.2.0", b: "v0.10.0", want: -1},
		{name: "pre-release suffix ignored", a: "v1.2.3-rc1", b: "v1.2.3", want: 0},
		{name: "minor newer", a: "v0.2.0", b: "v0.1.99", want: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CompareVersions(tc.a, tc.b)
			if err != nil {
				t.Fatalf("CompareVersions(%q, %q) returned error: %v", tc.a, tc.b, err)
			}
			if got != tc.want {
				t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestCompareVersionsRejectsNonSemver(t *testing.T) {
	invalid := []string{"dev", "", "not-a-version", "v1.x.0"}
	for _, v := range invalid {
		if _, err := CompareVersions(v, "v0.1.0"); err == nil {
			t.Errorf("CompareVersions(%q, ...) expected error, got nil", v)
		}
		if _, err := CompareVersions("v0.1.0", v); err == nil {
			t.Errorf("CompareVersions(..., %q) expected error, got nil", v)
		}
	}
}
