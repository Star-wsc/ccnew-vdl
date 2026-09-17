package safeurl

import "testing"

func TestURLHostMatches(t *testing.T) {
	if URLHostMatches("http://evil.com/?x=b23.tv", "b23.tv") {
		t.Fatal("query spoof should fail")
	}
	if !URLHostMatches("https://b23.tv/abc", "b23.tv") {
		t.Fatal("real short link should pass")
	}
	if !URLHostMatches("https://www.bilibili.com/video/BV1", "bilibili.com") {
		t.Fatal("www.bilibili.com should pass")
	}
	if URLHostMatches("http://bilibili.com.evil.com/x", "bilibili.com") {
		t.Fatal("suffix spoof should fail")
	}
	if !URLHostMatches("https://upcdn.example.bilivideo.com/v", "bilivideo.com") {
		t.Fatal("subdomain bilivideo should pass")
	}
}

func TestMediaAndPlatform(t *testing.T) {
	if !IsBilibiliShortLink("https://b23.tv/xyz") {
		t.Fatal("b23")
	}
	if IsBilibiliShortLink("https://127.0.0.1/admin?b23.tv") {
		t.Fatal("ssrf shortlink")
	}
	if IsBilibiliMediaURL("http://attacker.tld/log?bilibili.com") {
		t.Fatal("cookie exfil host")
	}
	if !IsBilibiliMediaURL("https://upos-sz-mirror.bilivideo.com/x.mp4") {
		t.Fatal("real bilivideo")
	}
	if IsTrustedDownloadURL("http://169.254.169.254/latest/meta-data/") {
		t.Fatal("metadata url")
	}
	if !IsTrustedDownloadURL("https://v26-web.douyinvod.com/xxx/video.mp4") {
		t.Fatal("douyinvod")
	}
}
