package douyin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type CollectionParser struct {
	client  *http.Client
	cookies string
}

func NewCollectionParser() *CollectionParser {
	return &CollectionParser{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *CollectionParser) SetCookies(cookies string) {
	p.cookies = cookies
}

type CollectionInfo struct {
	ID         string
	Title      string
	Author     string
	CoverURL   string
	TotalCount int
	Videos     []*CollectionVideoInfo
}

type CollectionVideoInfo struct {
	VideoID  string
	URL      string
	Title    string
	Author   string
	CoverURL string
	Duration int
	Page     int
}

// ParseCollection 解析抖音合集URL，返回合集信息
func (p *CollectionParser) ParseCollection(urlStr string) (*CollectionInfo, error) {
	// 如果是短链接，先解析获取真实URL
	resolvedURL := urlStr
	if strings.Contains(urlStr, "v.douyin.com") {
		realURL, err := resolveShortURL(urlStr)
		if err != nil {
			return nil, fmt.Errorf("解析短链接失败: %v", err)
		}
		resolvedURL = realURL
	}

	// 提取合集ID
	collectionID, contentType, err := extractCollectionID(resolvedURL)
	if err != nil {
		return nil, fmt.Errorf("无法识别的合集URL格式: %s", urlStr)
	}

	// 根据类型选择不同的API
	var info *CollectionInfo
	if contentType == "playlet" {
		info, err = p.fetchPlayletByAPI(collectionID)
	} else {
		info, err = p.fetchCollectionByAPI(collectionID)
	}

	if err != nil {
		// 尝试HTML解析
		htmlURL := resolvedURL
		if strings.Contains(resolvedURL, "iesdouyin.com") {
			htmlURL = fmt.Sprintf("https://www.douyin.com/collection/%s", collectionID)
		}
		info, err = p.fetchCollectionFromHTML(collectionID, htmlURL)
		if err != nil {
			return nil, fmt.Errorf("合集解析失败: %w", err)
		}
	}

	return info, nil
}

func resolveShortURL(shortURL string) (string, error) {
	req, err := http.NewRequest("GET", shortURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 13; SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36")

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 301 || resp.StatusCode == 302 {
		location := resp.Header.Get("Location")
		if location != "" {
			return location, nil
		}
	}

	return "", fmt.Errorf("no redirect")
}

func extractCollectionID(urlStr string) (string, string, error) {
	// 匹配 douyin.com/collection/xxx
	re := regexp.MustCompile(`douyin\.com/collection/(\d+)`)
	matches := re.FindStringSubmatch(urlStr)
	if len(matches) > 1 {
		return matches[1], "collection", nil
	}

	// 匹配 collection=xxx
	re2 := regexp.MustCompile(`collection=(\d+)`)
	matches2 := re2.FindStringSubmatch(urlStr)
	if len(matches2) > 1 {
		return matches2[1], "collection", nil
	}

	// 匹配 playlet/detail/xxx (短剧)
	re3 := regexp.MustCompile(`playlet/detail/(\d+)`)
	matches3 := re3.FindStringSubmatch(urlStr)
	if len(matches3) > 1 {
		return matches3[1], "playlet", nil
	}

	// 匹配 object_id=xxx
	re4 := regexp.MustCompile(`object_id=(\d+)`)
	matches4 := re4.FindStringSubmatch(urlStr)
	if len(matches4) > 1 {
		return matches4[1], "playlet", nil
	}

	return "", "", fmt.Errorf("无法提取合集ID")
}

func (p *CollectionParser) fetchPlayletByAPI(mixID string) (*CollectionInfo, error) {
	apiURL := fmt.Sprintf("https://www.douyin.com/aweme/v1/web/mix/item/list/?mix_id=%s&cursor=0&count=20&aid=6383", mixID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if len(body) == 0 || body[0] == '<' {
		return nil, fmt.Errorf("API返回非JSON")
	}

	var result struct {
		StatusCode int    `json:"status_code"`
		AwemeList  []struct {
			AwemeID string `json:"aweme_id"`
			Desc    string `json:"desc"`
			Author  struct {
				Nickname string `json:"nickname"`
			} `json:"author"`
			Video struct {
				Cover struct {
					URLList []string `json:"url_list"`
				} `json:"cover"`
				Duration int `json:"duration"`
			} `json:"video"`
			ShareInfo struct {
				ShareURL string `json:"share_url"`
			} `json:"share_info"`
		} `json:"aweme_list"`
		MixInfo struct {
			MixName string `json:"mix_name"`
		} `json:"mix_info"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.StatusCode != 0 || len(result.AwemeList) == 0 {
		return nil, fmt.Errorf("API返回错误或无数据")
	}

	title := result.MixInfo.MixName
	if title == "" {
		title = fmt.Sprintf("抖音短剧_%s", mixID)
	}

	videos := make([]*CollectionVideoInfo, 0, len(result.AwemeList))
	for i, aweme := range result.AwemeList {
		coverURL := ""
		if len(aweme.Video.Cover.URLList) > 0 {
			coverURL = aweme.Video.Cover.URLList[0]
		}
		shareURL := aweme.ShareInfo.ShareURL
		if shareURL == "" {
			shareURL = fmt.Sprintf("https://www.douyin.com/video/%s", aweme.AwemeID)
		}
		videos = append(videos, &CollectionVideoInfo{
			VideoID:  aweme.AwemeID,
			URL:      shareURL,
			Title:    aweme.Desc,
			Author:   aweme.Author.Nickname,
			CoverURL: coverURL,
			Duration: aweme.Video.Duration / 1000,
			Page:     i + 1,
		})
	}

	return &CollectionInfo{
		ID:         fmt.Sprintf("dy_%s", mixID),
		Title:      title,
		TotalCount: len(videos),
		Videos:     videos,
	}, nil
}

func (p *CollectionParser) fetchCollectionByAPI(collectionID string) (*CollectionInfo, error) {
	apiURL := fmt.Sprintf("https://www.douyin.com/aweme/v1/web/aweme/listcollection/?collection_id=%s&cursor=0&count=20&aid=6383", collectionID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if len(body) == 0 || body[0] == '<' {
		return nil, fmt.Errorf("API返回非JSON")
	}

	var result struct {
		StatusCode int    `json:"status_code"`
		AwemeList  []struct {
			AwemeID string `json:"aweme_id"`
			Desc    string `json:"desc"`
			Author  struct {
				Nickname string `json:"nickname"`
			} `json:"author"`
			Video struct {
				Cover struct {
					URLList []string `json:"url_list"`
				} `json:"cover"`
				Duration int `json:"duration"`
			} `json:"video"`
			ShareInfo struct {
				ShareURL string `json:"share_url"`
			} `json:"share_info"`
		} `json:"aweme_list"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.StatusCode != 0 || len(result.AwemeList) == 0 {
		return nil, fmt.Errorf("API返回错误或无数据")
	}

	videos := make([]*CollectionVideoInfo, 0, len(result.AwemeList))
	for i, aweme := range result.AwemeList {
		coverURL := ""
		if len(aweme.Video.Cover.URLList) > 0 {
			coverURL = aweme.Video.Cover.URLList[0]
		}
		shareURL := aweme.ShareInfo.ShareURL
		if shareURL == "" {
			shareURL = fmt.Sprintf("https://www.douyin.com/video/%s", aweme.AwemeID)
		}
		videos = append(videos, &CollectionVideoInfo{
			VideoID:  aweme.AwemeID,
			URL:      shareURL,
			Title:    aweme.Desc,
			Author:   aweme.Author.Nickname,
			CoverURL: coverURL,
			Duration: aweme.Video.Duration / 1000,
			Page:     i + 1,
		})
	}

	return &CollectionInfo{
		ID:         fmt.Sprintf("dy_col_%s", collectionID),
		Title:      fmt.Sprintf("抖音合集_%s", collectionID),
		TotalCount: len(videos),
		Videos:     videos,
	}, nil
}

func (p *CollectionParser) fetchCollectionFromHTML(collectionID, urlStr string) (*CollectionInfo, error) {
	pageURL := urlStr
	if pageURL == "" {
		pageURL = fmt.Sprintf("https://www.douyin.com/collection/%s", collectionID)
	}

	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return nil, err
	}
	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)

	// 从HTML中提取视频链接
	videoRe := regexp.MustCompile(`href="[^"]*?/video/(\d+)"`)
	matches := videoRe.FindAllStringSubmatch(html, -1)

	if len(matches) == 0 {
		videoRe = regexp.MustCompile(`"aweme_id"\s*:\s*"(\d+)"`)
		matches = videoRe.FindAllStringSubmatch(html, -1)
	}

	seen := make(map[string]bool)
	var videos []*CollectionVideoInfo
	page := 0
	for _, match := range matches {
		if len(match) > 1 {
			videoID := match[1]
			if !seen[videoID] {
				seen[videoID] = true
				page++
				videos = append(videos, &CollectionVideoInfo{
					VideoID: videoID,
					URL:     fmt.Sprintf("https://www.douyin.com/video/%s", videoID),
					Page:    page,
				})
			}
		}
	}

	if len(videos) == 0 {
		return nil, fmt.Errorf("无法从HTML中提取视频列表")
	}

	return &CollectionInfo{
		ID:         fmt.Sprintf("dy_col_%s", collectionID),
		Title:      fmt.Sprintf("抖音合集_%s", collectionID),
		TotalCount: len(videos),
		Videos:     videos,
	}, nil
}

func (p *CollectionParser) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.douyin.com/")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	if p.cookies != "" {
		req.Header.Set("Cookie", p.cookies)
	}
}

// IsDouyinCollectionURL 检查URL是否是明确的抖音合集链接
func IsDouyinCollectionURL(urlStr string) bool {
	matched, _ := regexp.MatchString(`douyin\.com/collection/\d+`, urlStr)
	if matched {
		return true
	}
	matched, _ = regexp.MatchString(`douyin\.com/user/.*collection=\d+`, urlStr)
	if matched {
		return true
	}
	matched, _ = regexp.MatchString(`iesdouyin\.com/share/playlet/detail/\d+`, urlStr)
	return matched
}
