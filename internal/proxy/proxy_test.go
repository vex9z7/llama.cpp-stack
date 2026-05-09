package proxy

import "testing"

func TestJoinPath(t *testing.T) {
	cases := []struct{ a, b, want string }{{"", "/v1/models", "/v1/models"}, {"/base", "/v1/models", "/base/v1/models"}, {"/base/", "v1/models", "/base/v1/models"}}
	for _, tc := range cases {
		if got := joinPath(tc.a, tc.b); got != tc.want {
			t.Fatalf("joinPath(%q,%q)=%q want %q", tc.a, tc.b, got, tc.want)
		}
	}
}
