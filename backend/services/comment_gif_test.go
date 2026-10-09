package services

import "testing"

func TestValidCommentGifURL(t *testing.T) {
	ok := []string{
		"https://media2.giphy.com/media/abc/200.gif?cid=1&rid=200.gif",
		"https://i.giphy.com/abc.webp",
		"https://giphy.com/gifs/abc",
	}
	bad := []string{
		"",
		"http://media.giphy.com/media/abc/200.gif",
		"https://evil.com/media.giphy.com/x.gif",
		"https://giphy.com.evil.com/x.gif",
		"https://notgiphy.com/x.gif",
		"https://user@media.giphy.com/x.gif",
		"javascript:alert(1)",
	}
	for _, u := range ok {
		if !ValidCommentGifURL(u) {
			t.Errorf("expected valid: %s", u)
		}
	}
	for _, u := range bad {
		if ValidCommentGifURL(u) {
			t.Errorf("expected invalid: %s", u)
		}
	}
}
