package models

type Platform string

const (
	PlatformDouyin   Platform = "douyin"
	PlatformBilibili Platform = "bilibili"
	PlatformUnknown  Platform = "unknown"
)

type PlatformParser interface {
	Parse(videoURL, quality string) (*VideoInfo, error)
	GetPlatform() Platform
}

