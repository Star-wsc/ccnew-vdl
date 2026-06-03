package models

type CollectionInfo struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Author      string                 `json:"author"`
	CoverURL    string                 `json:"cover_url"`
	Description string                 `json:"description"`
	TotalCount  int                    `json:"total_count"`
	Videos      []*CollectionVideoInfo `json:"videos"`
}

type CollectionVideoInfo struct {
	BVID     string `json:"bvid"`
	AID      int    `json:"aid"`
	CID      int    `json:"cid"`
	VideoID  string `json:"video_id"` // 抖音 aweme_id
	URL      string `json:"url"`      // 原始视频URL
	Title    string `json:"title"`
	Author   string `json:"author"`
	CoverURL string `json:"cover_url"`
	Duration int    `json:"duration"`
	Page     int    `json:"page"`
}

type Collection struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	CoverURL    string `json:"cover_url"`
	Description string `json:"description"`
	TotalCount  int    `json:"total_count"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type CollectionVideo struct {
	ID           string `json:"id"`
	CollectionID string `json:"collection_id"`
	BVID         string `json:"bvid"`
	AID          int    `json:"aid"`
	CID          int    `json:"cid"`
	VideoID      string `json:"video_id"` // 抖音 aweme_id
	URL          string `json:"url"`      // 原始视频URL
	Title        string `json:"title"`
	Author       string `json:"author"`
	CoverURL     string `json:"cover_url"`
	Duration     int    `json:"duration"`
	Page         int    `json:"page"`
	Status       string `json:"status"`
	FilePath     string `json:"file_path"`
	FileSize     int64  `json:"file_size"`
	ErrorMessage string `json:"error_message"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type PreviewCollectionRequest struct {
	URL string `json:"url"`
}

type CreateCollectionRequest struct {
	URL          string                  `json:"url"`
	Title        string                  `json:"title"`
	Videos       []*CollectionVideoInfo  `json:"videos"`
	Quality      string                  `json:"quality"`
	AutoDownload bool                    `json:"auto_download"`
}
