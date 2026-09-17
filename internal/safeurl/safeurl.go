// Package safeurl 提供 URL 主机名精确匹配，避免 strings.Contains 误判导致 SSRF / Cookie 外带。
package safeurl

import (
	"net/url"
	"strings"
)

// HostFromURL 返回小写 hostname，解析失败返回空。
func HostFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// HostMatch host 是否等于 suffix，或为 suffix 的子域。
func HostMatch(host, suffix string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	suffix = strings.ToLower(strings.TrimSpace(suffix))
	if host == "" || suffix == "" {
		return false
	}
	return host == suffix || strings.HasSuffix(host, "."+suffix)
}

// URLHostMatches raw 的 hostname 是否命中任一后缀（精确域或子域）。
func URLHostMatches(raw string, suffixes ...string) bool {
	h := HostFromURL(raw)
	if h == "" {
		return false
	}
	for _, s := range suffixes {
		if HostMatch(h, s) {
			return true
		}
	}
	return false
}

// IsBilibiliShortLink 仅 b23.tv 官方短链域名。
func IsBilibiliShortLink(raw string) bool {
	return URLHostMatches(raw, "b23.tv")
}

// IsDouyinShortLink 抖音短链域名。
func IsDouyinShortLink(raw string) bool {
	return URLHostMatches(raw, "v.douyin.com")
}

// BilibiliWebHosts 判断「这是 B 站页面/合集链接」用的域名。
func IsBilibiliWebURL(raw string) bool {
	return URLHostMatches(raw, "bilibili.com", "b23.tv")
}

// IsDouyinWebURL 判断抖音页面/分享链接。
func IsDouyinWebURL(raw string) bool {
	return URLHostMatches(raw, "douyin.com", "iesdouyin.com", "v.douyin.com")
}

// IsYouTubeWebURL YouTube 页面链接。
func IsYouTubeWebURL(raw string) bool {
	return URLHostMatches(raw, "youtube.com", "youtu.be", "youtube-nocookie.com")
}

// IsBilibiliMediaURL 下载媒体/封面时是否允许挂 B 站 Cookie/Referer。
func IsBilibiliMediaURL(raw string) bool {
	return URLHostMatches(raw,
		"bilibili.com", "hdslb.com", "bilivideo.com", "bilivideo.cn", "biliapi.com")
}

// IsDouyinMediaURL 下载媒体时是否允许挂抖音 UA/Cookie。
func IsDouyinMediaURL(raw string) bool {
	return URLHostMatches(raw,
		"douyin.com", "iesdouyin.com", "douyinvod.com", "douyinpic.com",
		"douyinstatic.com", "byteimg.com", "bytecdn.cn", "zjcdn.com",
		"snssdk.com", "amemv.com")
}

// IsTrustedDownloadURL 预览链路里客户端传来的流地址是否允许直接下载。
// 仅允许已知平台 CDN/API 域，拒绝任意 http(s)。
func IsTrustedDownloadURL(raw string) bool {
	return IsBilibiliMediaURL(raw) || IsDouyinMediaURL(raw) ||
		URLHostMatches(raw,
			"ytimg.com", "ggpht.com", "googlevideo.com", "googleusercontent.com",
			"youtube.com")
}
