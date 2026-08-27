package main

import "testing"

func TestIsProxyURLAllowed(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"bilibili cover", "https://i0.hdslb.com/bfs/archive/abc.jpg", true},
		{"biliimg", "https://archive.biliimg.com/bfs/images/x.png", true},
		{"douyin cover", "https://p3-sign.douyinpic.com/tos-cn-i/x.jpeg", true},
		{"douyin vod", "https://v26-web.douyinvod.com/video/tos/cn/clip?a=1", true},
		{"byteimg", "https://p3-webcast-sign.byteimg.com/img/x.jpg", true},
		{"uppercase host", "HTTPS://I0.HDSLB.COM/A.JPG", true},
		{"lookalike suffix", "https://i0.hdslb.com.evil.com/a.jpg", false},
		{"plain evil", "https://evil.com/a.jpg", false},
		{"cloud metadata", "http://169.254.169.254/latest/meta-data/", false},
		{"localhost", "http://127.0.0.1:18000/api/tasks", false},
		{"lan ipv4", "http://192.168.1.10/cover.jpg", false},
		{"public ip literal", "https://104.16.0.1/a.jpg", false},
		{"file scheme", "file:///etc/passwd", false},
		{"garbage input", ":::", false},
		{"missing host", "https:///path", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isProxyURLAllowed(tc.url); got != tc.want {
				t.Fatalf("isProxyURLAllowed(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}
