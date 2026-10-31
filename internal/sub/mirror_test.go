package sub

import "testing"

func TestRewriteURLSkipsNonGitHub(t *testing.T) {
	for _, u := range []string{
		"https://example.com/rules.list",
		"https://cdn.jsdelivr.net/gh/a/b@main/rules.list",
		"",
	} {
		if got := RewriteURL(u); got != u {
			t.Fatalf("非 GitHub raw 链接不应改写：%s -> %s", u, got)
		}
	}
}

func TestIsGitHubRaw(t *testing.T) {
	if !IsGitHubRaw("https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/x/Clash/BanAD.list") {
		t.Fatal("应识别 raw.githubusercontent.com")
	}
	if IsGitHubRaw("https://api.github.com/repos/x/y") {
		t.Fatal("api.github.com 不应识别为 raw")
	}
}
