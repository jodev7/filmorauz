package repositories

import "testing"

func TestFingerprintGroupsSameErrorAcrossBuilds(t *testing.T) {
	a := ErrorEvent{Source: "client", Kind: "window", Message: "TypeError: x is undefined",
		Stack: "TypeError: x is undefined\n    at render (https://filmorauz.net/_next/static/chunks/page.js?v=1:10:5)"}
	b := a
	b.Stack = "TypeError: x is undefined\n    at render (https://filmorauz.net/_next/static/chunks/page.js?v=2:10:5)"
	b.URL = "https://filmorauz.net/movies/other"
	if Fingerprint(a) != Fingerprint(b) {
		t.Error("same error on different URLs / cache-busted chunks should group together")
	}
	c := a
	c.Message = "TypeError: y is undefined"
	if Fingerprint(a) == Fingerprint(c) {
		t.Error("different messages must not group")
	}
	d := a
	d.Source = "server"
	if Fingerprint(a) == Fingerprint(d) {
		t.Error("client and server errors must not group")
	}
}
