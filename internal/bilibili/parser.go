package bilibili

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Parser struct {
	client  *http.Client
	cookies string
}

func NewParser() *Parser {
	return &Parser{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *Parser) SetCookies(cookies string) {
	p.cookies = cookies
}

type VideoInfo struct {
	BVID      string
	AID       int64
	CID       int64
	Title     string
	Author    string
	CoverURL  string
	VideoURL  string
	AudioURL  string
	Quality   string
	Duration  int
}

func (p *Parser) Parse(url string, quality string) (*VideoInfo, error) {
	bvid := extractBVID(url)
	if bvid == "" {
		return nil, fmt.Errorf("无法提取BVID: %s", url)
	}

	info, err := p.getVideoInfo(bvid)
	if err != nil {
		return nil, fmt.Errorf("获取视频信息失败: %w", err)
	}

	videoURL, audioURL, actualQn, err := p.getVideoURLs(info.AID, info.CID, quality)
	if err != nil {
		return nil, fmt.Errorf("获取视频流失败: %w", err)
	}

	info.VideoURL = videoURL
	info.AudioURL = audioURL
	info.Quality = qualityName(actualQn)
	return info, nil
}

func extractBVID(url string) string {
	re := regexp.MustCompile(`BV[a-zA-Z0-9]+`)
	match := re.FindString(url)
	return match
}

func (p *Parser) getVideoInfo(bvid string) (*VideoInfo, error) {
	apiURL := fmt.Sprintf("https://api.bilibili.com/x/web-interface/view?bvid=%s", bvid)
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

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			BVID   string `json:"bvid"`
			AID    int64  `json:"aid"`
			CID    int64  `json:"cid"`
			Title  string `json:"title"`
			Owner  struct {
				Name string `json:"name"`
			} `json:"owner"`
			Pic      string `json:"pic"`
			Duration int    `json:"duration"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API错误: %d %s", result.Code, result.Msg)
	}

	return &VideoInfo{
		BVID:     result.Data.BVID,
		AID:      result.Data.AID,
		CID:      result.Data.CID,
		Title:    result.Data.Title,
		Author:   result.Data.Owner.Name,
		CoverURL: result.Data.Pic,
		Duration: result.Data.Duration,
	}, nil
}

func (p *Parser) getVideoURLs(aid, cid int64, quality string) (videoURL, audioURL string, actualQn int, err error) {
	qn := qualityToQn(quality)
	apiURL := fmt.Sprintf("https://api.bilibili.com/x/player/playurl?avid=%d&cid=%d&qn=%d&fnval=16&fourk=1", aid, cid, qn)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", "", 0, err
	}

	p.setHeaders(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", 0, err
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			Durl []struct {
				URL string `json:"url"`
			} `json:"durl"`
			Dash struct {
				Video []struct {
					ID   int    `json:"id"`
					BaseURL string `json:"baseUrl"`
				} `json:"video"`
				Audio []struct {
					BaseURL string `json:"baseUrl"`
				} `json:"audio"`
			} `json:"dash"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", 0, err
	}

	if result.Code != 0 {
		return "", "", 0, fmt.Errorf("playurl API错误: %d", result.Code)
	}

	// DASH格式
	if len(result.Data.Dash.Video) > 0 {
		// 选择最高画质（遍历所有流，取 ID 最大的）
		var selectedVideo struct {
			ID   int    `json:"id"`
			BaseURL string `json:"baseUrl"`
		}
		for _, v := range result.Data.Dash.Video {
			if v.ID > selectedVideo.ID {
				selectedVideo = v
			}
		}
		// 如果没有找到更好的，使用第一个
		if selectedVideo.BaseURL == "" && len(result.Data.Dash.Video) > 0 {
			selectedVideo = result.Data.Dash.Video[0]
		}

		if len(result.Data.Dash.Audio) > 0 {
			return selectedVideo.BaseURL, result.Data.Dash.Audio[0].BaseURL, selectedVideo.ID, nil
		}
		return selectedVideo.BaseURL, "", selectedVideo.ID, nil
	}

	// Durl格式
	if len(result.Data.Durl) > 0 {
		return result.Data.Durl[0].URL, "", qn, nil
	}

	return "", "", 0, fmt.Errorf("未找到视频流")
}

func (p *Parser) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.bilibili.com")
	req.Header.Set("Origin", "https://www.bilibili.com")
	if p.cookies != "" {
		req.Header.Set("Cookie", p.cookies)
	}
}

func qualityToQn(quality string) int {
	switch strings.ToLower(quality) {
	case "4k":
		return 120
	case "1080p":
		return 80
	case "720p":
		return 64
	case "480p":
		return 32
	default:
		return 80
	}
}

func qualityName(qn int) string {
	switch qn {
	case 120:
		return "4K"
	case 80:
		return "1080P"
	case 64:
		return "720P"
	case 32:
		return "480P"
	default:
		return fmt.Sprintf("%d", qn)
	}
}
