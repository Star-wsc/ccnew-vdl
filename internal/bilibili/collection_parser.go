package bilibili

import (
	"encoding/json"
	"log"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Star-wsc/ccnew-vdl/internal/models"
)

type CollectionParser struct {
	client  *http.Client
	cookies string
}

func NewCollectionParser() *CollectionParser {
	return &CollectionParser{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *CollectionParser) SetCookies(cookies string) {
	p.cookies = cookies
}

type CollectionURLType string

const (
	CollectionURLTypeSeason    CollectionURLType = "season"
	CollectionURLTypeSeries    CollectionURLType = "series"
	CollectionURLTypeMediaList CollectionURLType = "medialist"
	CollectionURLTypeChannel   CollectionURLType = "channel"
	CollectionURLTypeSpaceList CollectionURLType = "spacelist"
)

type ParsedCollectionURL struct {
	Type     CollectionURLType
	SeasonID int
	SeriesID int
	MediaID  int
	UpID     int
	SID      int
}

func (p *CollectionParser) ParseCollectionURL(urlStr string) (*ParsedCollectionURL, error) {
	seasonPattern := regexp.MustCompile(`bilibili\.com/bangumi/play/ss(\d+)`)
	seasonPattern2 := regexp.MustCompile(`bilibili\.com/bangumi/media/md(\d+)`)
	seriesPattern := regexp.MustCompile(`bilibili\.com/medialist/play/ml(\d+)`)
	channelPattern := regexp.MustCompile(`space\.bilibili\.com/(\d+)/channel/collectiondetail\?sid=(\d+)`)
	channelPattern2 := regexp.MustCompile(`space\.bilibili\.com/(\d+)/channel/seriesdetail\?sid=(\d+)`)
	listPattern := regexp.MustCompile(`bilibili\.com/list/(\d+)\?sid=(\d+)`)
	spaceListsPattern := regexp.MustCompile(`space\.bilibili\.com/(\d+)/lists/(\d+)`)
	videoListPattern := regexp.MustCompile(`bilibili\.com/video/(BV[a-zA-Z0-9]+)(?:/.*\?p=(\d+))?`)

	log.Printf("ParseCollectionURL: urlStr=%s", urlStr)

	if matches := spaceListsPattern.FindStringSubmatch(urlStr); len(matches) > 2 {
		upID, _ := strconv.Atoi(matches[1])
		sid, _ := strconv.Atoi(matches[2])
		log.Printf("spaceListsPattern matched: upID=%d, sid=%d", upID, sid)
		return &ParsedCollectionURL{Type: CollectionURLTypeSpaceList, UpID: upID, SID: sid}, nil
	}

	if matches := seasonPattern.FindStringSubmatch(urlStr); len(matches) > 1 {
		seasonID, _ := strconv.Atoi(matches[1])
		return &ParsedCollectionURL{Type: CollectionURLTypeSeason, SeasonID: seasonID}, nil
	}

	if matches := seasonPattern2.FindStringSubmatch(urlStr); len(matches) > 1 {
		mediaID, _ := strconv.Atoi(matches[1])
		return &ParsedCollectionURL{Type: CollectionURLTypeSeason, MediaID: mediaID}, nil
	}

	if matches := seriesPattern.FindStringSubmatch(urlStr); len(matches) > 1 {
		mediaID, _ := strconv.Atoi(matches[1])
		return &ParsedCollectionURL{Type: CollectionURLTypeMediaList, MediaID: mediaID}, nil
	}

	if matches := channelPattern.FindStringSubmatch(urlStr); len(matches) > 2 {
		upID, _ := strconv.Atoi(matches[1])
		sid, _ := strconv.Atoi(matches[2])
		return &ParsedCollectionURL{Type: CollectionURLTypeChannel, UpID: upID, SID: sid}, nil
	}

	if matches := channelPattern2.FindStringSubmatch(urlStr); len(matches) > 2 {
		upID, _ := strconv.Atoi(matches[1])
		sid, _ := strconv.Atoi(matches[2])
		return &ParsedCollectionURL{Type: CollectionURLTypeSeries, UpID: upID, SID: sid}, nil
	}

	if matches := listPattern.FindStringSubmatch(urlStr); len(matches) > 2 {
		upID, _ := strconv.Atoi(matches[1])
		sid, _ := strconv.Atoi(matches[2])
		return &ParsedCollectionURL{Type: CollectionURLTypeSeries, UpID: upID, SID: sid}, nil
	}

	if matches := videoListPattern.FindStringSubmatch(urlStr); len(matches) > 1 {
		return p.parseVideoCollection(urlStr, matches[1])
	}

	return nil, fmt.Errorf("无法识别的合集URL格式: %s", urlStr)
}

func (p *CollectionParser) parseVideoCollection(urlStr, bvid string) (*ParsedCollectionURL, error) {
	apiURL := fmt.Sprintf("https://api.bilibili.com/x/web-interface/view?bvid=%s", bvid)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	p.setCommonHeaders(req)

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
		Code int `json:"code"`
		Data struct {
			Aid   int `json:"aid"`
			Pages []struct {
				Cid  int `json:"cid"`
				Page int `json:"page"`
			} `json:"pages"`
			UgcSeason struct {
				ID    int    `json:"id"`
				Title string `json:"title"`
			} `json:"ugc_season"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API返回错误码: %d", result.Code)
	}

	if len(result.Data.Pages) > 1 {
		return &ParsedCollectionURL{Type: CollectionURLTypeSeries, SeriesID: result.Data.Aid, SID: result.Data.UgcSeason.ID}, nil
	}

	return nil, fmt.Errorf("该视频不是合集")
}

func (p *CollectionParser) ParseCollection(urlStr string) (*models.CollectionInfo, error) {
	// 短链接(b23.tv)先跟随跳转拿到真实URL
	if strings.Contains(urlStr, "b23.tv") {
		resolved, err := ResolveShortURL(urlStr)
		if err == nil && resolved != "" {
			log.Printf("[B站合集] 短链接解析为: %s", resolved)
			urlStr = resolved
		} else {
			log.Printf("[B站合集] 短链接解析失败: %v", err)
		}
	}

	parsedURL, err := p.ParseCollectionURL(urlStr)
	if err != nil {
		return nil, err
	}

	log.Printf("ParseCollection: URLType=%s, UpID=%d, SID=%d", parsedURL.Type, parsedURL.UpID, parsedURL.SID)

	switch parsedURL.Type {
	case CollectionURLTypeSeason:
		return p.parseSeasonCollection(parsedURL)
	case CollectionURLTypeMediaList:
		return p.parseMediaListCollection(parsedURL)
	case CollectionURLTypeSeries, CollectionURLTypeChannel:
		return p.parseSeriesCollection(parsedURL)
	case CollectionURLTypeSpaceList:
		return p.parseSpaceListCollection(parsedURL)
	default:
		return nil, fmt.Errorf("不支持的合集类型")
	}
}

func (p *CollectionParser) parseSeasonCollection(parsedURL *ParsedCollectionURL) (*models.CollectionInfo, error) {
	var apiURL string
	if parsedURL.SeasonID > 0 {
		apiURL = fmt.Sprintf("https://api.bilibili.com/pgc/view/web/season?season_id=%d", parsedURL.SeasonID)
	} else if parsedURL.MediaID > 0 {
		apiURL = fmt.Sprintf("https://api.bilibili.com/pgc/view/web/season?media_id=%d", parsedURL.MediaID)
	} else {
		return nil, fmt.Errorf("无效的番剧ID")
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	p.setCommonHeaders(req)

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
		Code int `json:"code"`
		Result struct {
			Title    string `json:"title"`
			Cover    string `json:"cover"`
			Evaluate string `json:"evaluate"`
			UpInfo   struct {
				Name string `json:"name"`
			} `json:"up_info"`
			Episodes []struct {
				ID        int    `json:"id"`
				Aid       int    `json:"aid"`
				Cid       int    `json:"cid"`
				Title     string `json:"title"`
				Cover     string `json:"cover"`
				ShareCopy string `json:"share_copy"`
			} `json:"episodes"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API返回错误码: %d", result.Code)
	}

	info := &models.CollectionInfo{
		ID:          fmt.Sprintf("season_%d", parsedURL.SeasonID),
		Title:       result.Result.Title,
		Author:      result.Result.UpInfo.Name,
		CoverURL:    result.Result.Cover,
		Description: result.Result.Evaluate,
		TotalCount:  len(result.Result.Episodes),
		Videos:      make([]*models.CollectionVideoInfo, 0),
	}

	for i, ep := range result.Result.Episodes {
		info.Videos = append(info.Videos, &models.CollectionVideoInfo{
			BVID:     fmt.Sprintf("ep%d", ep.ID),
			AID:      ep.Aid,
			CID:      ep.Cid,
			Title:    ep.Title,
			Author:   result.Result.UpInfo.Name,
			CoverURL: ep.Cover,
			Page:     i + 1,
		})
	}

	return info, nil
}

func (p *CollectionParser) parseMediaListCollection(parsedURL *ParsedCollectionURL) (*models.CollectionInfo, error) {
	apiURL := fmt.Sprintf("https://api.bilibili.com/x/v1/medialist/info?type=1&biz_id=%d&tid=0", parsedURL.MediaID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	p.setCommonHeaders(req)

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
		Code int `json:"code"`
		Data struct {
			Title      string `json:"title"`
			Cover      string `json:"cover"`
			Intro      string `json:"intro"`
			MediaCount int    `json:"media_count"`
			Upper      struct {
				Name string `json:"name"`
			} `json:"upper"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API返回错误码: %d", result.Code)
	}

	info := &models.CollectionInfo{
		ID:          fmt.Sprintf("ml%d", parsedURL.MediaID),
		Title:       result.Data.Title,
		Author:      result.Data.Upper.Name,
		CoverURL:    result.Data.Cover,
		Description: result.Data.Intro,
		TotalCount:  result.Data.MediaCount,
		Videos:      make([]*models.CollectionVideoInfo, 0),
	}

	videos, err := p.getMediaListVideos(parsedURL.MediaID)
	if err != nil {
		return nil, err
	}
	info.Videos = videos

	return info, nil
}

func (p *CollectionParser) getMediaListVideos(mediaID int) ([]*models.CollectionVideoInfo, error) {
	var videos []*models.CollectionVideoInfo
	pageSize := 20
	pageNum := 1

	for {
		apiURL := fmt.Sprintf("https://api.bilibili.com/x/v1/medialist/resource/list?type=1&biz_id=%d&ps=%d&pn=%d", mediaID, pageSize, pageNum)

		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, err
		}
		p.setCommonHeaders(req)

		resp, err := p.client.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		var result struct {
			Code int `json:"code"`
			Data struct {
				MediaList []struct {
					ID    int    `json:"id"`
					Title string `json:"title"`
					Cover string `json:"cover"`
					Upper struct {
						Name string `json:"name"`
					} `json:"upper"`
					Page int `json:"page"`
				} `json:"media_list"`
				HasMore bool `json:"has_more"`
			} `json:"data"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}

		if result.Code != 0 {
			return nil, fmt.Errorf("API返回错误码: %d", result.Code)
		}

		for i, m := range result.Data.MediaList {
			bvid, aid, cid, err := p.getVideoInfoByAID(m.ID)
			if err != nil {
				continue
			}
			videos = append(videos, &models.CollectionVideoInfo{
				BVID:     bvid,
				AID:      aid,
				CID:      cid,
				Title:    m.Title,
				Author:   m.Upper.Name,
				CoverURL: m.Cover,
				Page:     (pageNum-1)*pageSize + i + 1,
			})
		}

		if !result.Data.HasMore {
			break
		}
		pageNum++
		time.Sleep(100 * time.Millisecond)
	}

	return videos, nil
}

func (p *CollectionParser) parseSeriesCollection(parsedURL *ParsedCollectionURL) (*models.CollectionInfo, error) {
	if parsedURL.SID <= 0 {
		return nil, fmt.Errorf("无效的系列ID")
	}

	apiURL := fmt.Sprintf("https://api.bilibili.com/x/polymer/space/seasons_archives_list?mid=%d&season_id=%d&sort_reverse=false&page_num=1&page_size=50", parsedURL.UpID, parsedURL.SID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	p.setCommonHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if len(body) > 0 && body[0] == '<' {
		return nil, fmt.Errorf("API返回HTML，可能需要登录或Cookie已过期")
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Meta struct {
				Name       string `json:"name"`
				Cover      string `json:"cover"`
				TotalCount int    `json:"total"`
			} `json:"meta"`
			Archives []struct {
				Aid   int    `json:"aid"`
				Bvid  string `json:"bvid"`
				Title string `json:"title"`
				Cover string `json:"pic"`
				Owner struct {
					Name string `json:"name"`
				} `json:"owner"`
				Pages []struct {
					Cid  int `json:"cid"`
					Page int `json:"page"`
				} `json:"pages"`
			} `json:"archives"`
			Page struct {
				Num   int `json:"num"`
				Size  int `json:"size"`
				Total int `json:"total"`
			} `json:"page"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w (响应前100字符: %s)", err, string(body[:min(100, len(body))]))
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API返回错误: code=%d, message=%s", result.Code, result.Message)
	}

	info := &models.CollectionInfo{
		ID:          fmt.Sprintf("series_%d", parsedURL.SID),
		Title:       result.Data.Meta.Name,
		Author:      "",
		CoverURL:    result.Data.Meta.Cover,
		Description: "",
		TotalCount:  result.Data.Meta.TotalCount,
		Videos:      make([]*models.CollectionVideoInfo, 0),
	}

	videoIndex := 0
	for _, archive := range result.Data.Archives {
		authorName := ""
		if archive.Owner.Name != "" {
			authorName = archive.Owner.Name
		}

		if len(archive.Pages) == 0 {
			videoIndex++
			info.Videos = append(info.Videos, &models.CollectionVideoInfo{
				BVID:     archive.Bvid,
				AID:      archive.Aid,
				CID:      0,
				Title:    archive.Title,
				Author:   authorName,
				CoverURL: archive.Cover,
				Page:     videoIndex,
			})
		} else if len(archive.Pages) == 1 {
			videoIndex++
			info.Videos = append(info.Videos, &models.CollectionVideoInfo{
				BVID:     archive.Bvid,
				AID:      archive.Aid,
				CID:      archive.Pages[0].Cid,
				Title:    archive.Title,
				Author:   authorName,
				CoverURL: archive.Cover,
				Page:     videoIndex,
			})
		} else {
			for _, page := range archive.Pages {
				videoIndex++
				info.Videos = append(info.Videos, &models.CollectionVideoInfo{
					BVID:     archive.Bvid,
					AID:      archive.Aid,
					CID:      page.Cid,
					Title:    fmt.Sprintf("%s - P%d", archive.Title, page.Page),
					Author:   authorName,
					CoverURL: archive.Cover,
					Page:     videoIndex,
				})
			}
		}
	}

	if len(result.Data.Archives) > 0 && result.Data.Archives[0].Owner.Name != "" {
		info.Author = result.Data.Archives[0].Owner.Name
	}

	return info, nil
}

func (p *CollectionParser) parseSpaceListCollection(parsedURL *ParsedCollectionURL) (*models.CollectionInfo, error) {
	if parsedURL.UpID <= 0 || parsedURL.SID <= 0 {
		return nil, fmt.Errorf("无效的合集ID: UpID=%d, SID=%d", parsedURL.UpID, parsedURL.SID)
	}

	log.Printf("parseSpaceListCollection: UpID=%d, SID=%d", parsedURL.UpID, parsedURL.SID)

	info := &models.CollectionInfo{
		ID:          fmt.Sprintf("spacelist_%d_%d", parsedURL.UpID, parsedURL.SID),
		Title:       "",
		Author:      "",
		CoverURL:    "",
		Description: "",
		TotalCount:  0,
		Videos:      make([]*models.CollectionVideoInfo, 0),
	}

	upInfoURL := fmt.Sprintf("https://api.bilibili.com/x/web-interface/card?mid=%d", parsedURL.UpID)
	req, err := http.NewRequest("GET", upInfoURL, nil)
	if err == nil {
		p.setCommonHeaders(req)
		resp, err := p.client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err == nil {
				var upResult struct {
					Code int `json:"code"`
					Data struct {
						Card struct {
							Name string `json:"name"`
						} `json:"card"`
					} `json:"data"`
				}
				if err := json.Unmarshal(body, &upResult); err == nil && upResult.Code == 0 {
					info.Author = upResult.Data.Card.Name
					log.Printf("获取到UP主名字: %s", info.Author)
				}
			}
		}
	}

	log.Printf("使用API获取合集视频列表")
	return p.parseSpaceListByAPI(parsedURL, info)
}

func (p *CollectionParser) parseSpaceListByAPI(parsedURL *ParsedCollectionURL, info *models.CollectionInfo) (*models.CollectionInfo, error) {
	log.Printf("parseSpaceListByAPI: UpID=%d, SID=%d", parsedURL.UpID, parsedURL.SID)

	signer := NewWBISigner()

	params := map[string]interface{}{
		"mid":          parsedURL.UpID,
		"season_id":    parsedURL.SID,
		"sort_reverse": false,
		"page_num":     1,
		"page_size":    50,
	}

	signParams, err := signer.Sign(params)
	if err != nil {
		log.Printf("WBI签名失败: %v，尝试不带签名", err)
	} else {
		for k, v := range signParams {
			params[k] = v
		}
	}

	apiURLs := []string{
		fmt.Sprintf("https://api.bilibili.com/x/polymer/web-space/seasons_archives_list?%s", buildQuery(params)),
		fmt.Sprintf("https://api.bilibili.com/x/polymer/space/seasons_archives_list?%s", buildQuery(params)),
		fmt.Sprintf("https://api.bilibili.com/x/space/season/archives_list?%s", buildQuery(params)),
	}

	var lastErr error
	for i, apiURL := range apiURLs {
		log.Printf("尝试API %d: %s", i+1, apiURL)

		result, err := p.tryAPIEndpoint(apiURL, parsedURL, info)
		if err != nil {
			log.Printf("API %d 失败: %v", i+1, err)
			lastErr = err
			continue
		}

		if len(result.Videos) > 0 {
			log.Printf("API %d 成功，获取到 %d 个视频", i+1, len(result.Videos))
			return result, nil
		}
	}

	log.Printf("所有API都失败，尝试从HTML页面提取")
	htmlResult, htmlErr := p.parseSpaceListFromHTML(parsedURL, info)
	if htmlErr == nil && len(htmlResult.Videos) > 0 {
		return htmlResult, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("所有解析方法失败，最后错误: %v", lastErr)
	}
	return nil, fmt.Errorf("无法解析合集，请检查链接是否正确")
}

func buildQuery(params map[string]interface{}) string {
	values := url.Values{}
	for k, v := range params {
		values.Add(k, fmt.Sprintf("%v", v))
	}
	return values.Encode()
}

func (p *CollectionParser) tryAPIEndpoint(apiURL string, parsedURL *ParsedCollectionURL, info *models.CollectionInfo) (*models.CollectionInfo, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	p.setCommonHeaders(req)
	req.Header.Set("Referer", fmt.Sprintf("https://space.bilibili.com/%d/lists/%d", parsedURL.UpID, parsedURL.SID))

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("API响应状态码: %d, 响应长度: %d", resp.StatusCode, len(body))

	if len(body) == 0 {
		return nil, fmt.Errorf("API返回空响应")
	}

	if body[0] == '<' {
		return nil, fmt.Errorf("API返回HTML而非JSON")
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Meta struct {
				Name       string `json:"name"`
				Cover      string `json:"cover"`
				TotalCount int    `json:"total"`
			} `json:"meta"`
			Archives []struct {
				Aid      int    `json:"aid"`
				Bvid     string `json:"bvid"`
				Title    string `json:"title"`
				Cover    string `json:"pic"`
				Duration int    `json:"duration"`
				Owner    struct {
					Name string `json:"name"`
				} `json:"owner"`
				Pages []struct {
					Cid  int `json:"cid"`
					Page int `json:"page"`
				} `json:"pages"`
			} `json:"archives"`
			Page struct {
				Num   int `json:"num"`
				Size  int `json:"size"`
				Total int `json:"total"`
			} `json:"page"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w, 响应前200字符: %s", err, string(body[:min(200, len(body))]))
	}

	log.Printf("API返回码: %d, 消息: %s", result.Code, result.Message)

	if result.Code != 0 {
		return nil, fmt.Errorf("API错误码: %d, 消息: %s", result.Code, result.Message)
	}

	info.Title = result.Data.Meta.Name
	info.CoverURL = result.Data.Meta.Cover
	info.TotalCount = result.Data.Meta.TotalCount

	videoIndex := 0
	for _, archive := range result.Data.Archives {
		authorName := archive.Owner.Name
		if authorName == "" {
			authorName = info.Author
		}

		if len(archive.Pages) == 0 {
			videoIndex++
			info.Videos = append(info.Videos, &models.CollectionVideoInfo{
				BVID:     archive.Bvid,
				AID:      archive.Aid,
				CID:      0,
				Title:    archive.Title,
				Author:   authorName,
				CoverURL: archive.Cover,
				Duration: archive.Duration,
				Page:     videoIndex,
			})
		} else if len(archive.Pages) == 1 {
			videoIndex++
			info.Videos = append(info.Videos, &models.CollectionVideoInfo{
				BVID:     archive.Bvid,
				AID:      archive.Aid,
				CID:      archive.Pages[0].Cid,
				Title:    archive.Title,
				Author:   authorName,
				CoverURL: archive.Cover,
				Duration: archive.Duration,
				Page:     videoIndex,
			})
		} else {
			for _, page := range archive.Pages {
				videoIndex++
				info.Videos = append(info.Videos, &models.CollectionVideoInfo{
					BVID:     archive.Bvid,
					AID:      archive.Aid,
					CID:      page.Cid,
					Title:    fmt.Sprintf("%s - P%d", archive.Title, page.Page),
					Author:   authorName,
					CoverURL: archive.Cover,
					Duration: archive.Duration,
					Page:     videoIndex,
				})
			}
		}
	}

	if len(info.Videos) == 0 {
		return nil, fmt.Errorf("API返回的视频列表为空")
	}

	if info.Author == "" && len(result.Data.Archives) > 0 {
		info.Author = result.Data.Archives[0].Owner.Name
	}

	return info, nil
}

func (p *CollectionParser) parseSpaceListFromHTML(parsedURL *ParsedCollectionURL, info *models.CollectionInfo) (*models.CollectionInfo, error) {
	collectionPageURL := fmt.Sprintf("https://space.bilibili.com/%d/lists/%d?type=series", parsedURL.UpID, parsedURL.SID)
	log.Printf("请求合集页面: %s", collectionPageURL)

	req, err := http.NewRequest("GET", collectionPageURL, nil)
	if err != nil {
		return nil, err
	}
	p.setCommonHeaders(req)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;")

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
	log.Printf("合集页面HTML长度: %d, 响应状态码: %d", len(html), resp.StatusCode)

	if len(html) < 1000 || strings.Contains(html, "验证") || strings.Contains(html, "登录") {
		log.Printf("合集页面返回内容过短或需要登录")
		return nil, fmt.Errorf("合集页面需要登录或Cookie已过期，请先在设置中配置B站Cookie")
	}

	if info.Title == "" {
		titlePattern := regexp.MustCompile(`<title[^>]*>([^<]+)</title>`)
		titleMatches := titlePattern.FindStringSubmatch(html)
		if len(titleMatches) > 1 {
			title := titleMatches[1]
			title = regexp.MustCompile(`_哔哩哔哩_bilibili$`).ReplaceAllString(title, "")
			title = regexp.MustCompile(`_哔哩哔哩_bilibili`).ReplaceAllString(title, "")
			title = regexp.MustCompile(`的个人空间.*$`).ReplaceAllString(title, "")
			if title != "" {
				info.Title = strings.TrimSpace(title)
				log.Printf("从HTML标题提取到合集名称: %s", info.Title)
			}
		}
	}

	initialStateStart := strings.Index(html, "window.__INITIAL_STATE__=")
	if initialStateStart == -1 {
		initialStateStart = strings.Index(html, "window.__INITIAL_STATE__ =")
	}

	if initialStateStart != -1 {
		jsonStart := strings.Index(html[initialStateStart:], "{")
		if jsonStart != -1 {
			jsonStart += initialStateStart
			jsonEnd := findMatchingBrace(html, jsonStart)
			if jsonEnd > jsonStart {
				jsonStr := html[jsonStart : jsonEnd+1]
				log.Printf("找到__INITIAL_STATE__数据，长度: %d", len(jsonStr))
				var initialState map[string]interface{}
				if err := json.Unmarshal([]byte(jsonStr), &initialState); err != nil {
					log.Printf("__INITIAL_STATE__解析失败: %v", err)
				} else {
					log.Printf("__INITIAL_STATE__解析成功")
					if seasons, ok := initialState["seasons"].(map[string]interface{}); ok {
						if list, ok := seasons["list"].([]interface{}); ok && len(list) > 0 {
							for _, item := range list {
								if season, ok := item.(map[string]interface{}); ok {
									if id, ok := season["id"].(float64); ok && int(id) == parsedURL.SID {
										if title, ok := season["title"].(string); ok && title != "" {
											info.Title = title
											log.Printf("从__INITIAL_STATE__提取到合集标题: %s", info.Title)
										}
										if cover, ok := season["cover"].(string); ok {
											info.CoverURL = cover
										}
										if count, ok := season["count"].(float64); ok {
											info.TotalCount = int(count)
										}
										break
									}
								}
							}
						}
					}

					if archives, ok := initialState["archives"].(map[string]interface{}); ok {
						if list, ok := archives["list"].([]interface{}); ok {
							for _, item := range list {
								archive, ok := item.(map[string]interface{})
								if !ok {
									continue
								}

								bvid, _ := archive["bvid"].(string)
								if bvid == "" {
									continue
								}

								title, _ := archive["title"].(string)
								aid, _ := archive["aid"].(float64)
								duration, _ := archive["duration"].(float64)
								pic, _ := archive["pic"].(string)

								var ownerName string
								if owner, ok := archive["owner"].(map[string]interface{}); ok {
									ownerName, _ = owner["name"].(string)
								}
								if ownerName == "" {
									ownerName = info.Author
								}

								var cid int
								if pages, ok := archive["pages"].([]interface{}); ok && len(pages) > 0 {
									if page, ok := pages[0].(map[string]interface{}); ok {
										if c, ok := page["cid"].(float64); ok {
											cid = int(c)
										}
									}
								}

								log.Printf("从__INITIAL_STATE__提取视频: BVID=%s, 标题=%s", bvid, title)
								info.Videos = append(info.Videos, &models.CollectionVideoInfo{
									BVID:     bvid,
									AID:      int(aid),
									CID:      cid,
									Title:    title,
									Author:   ownerName,
									CoverURL: pic,
									Duration: int(duration),
									Page:     len(info.Videos) + 1,
								})
							}
						}
					}
				}
			}
		}
	}

	if len(info.Videos) == 0 {
		videoURLPattern := regexp.MustCompile(`href="/video/(BV[a-zA-Z0-9]{10})[^"]*"`)
		videoURLMatches := videoURLPattern.FindAllStringSubmatch(html, -1)

		uniqueBVIDs := make(map[string]bool)
		var orderedBVIDs []string
		for _, match := range videoURLMatches {
			if len(match) > 1 {
				bvid := match[1]
				if !uniqueBVIDs[bvid] {
					uniqueBVIDs[bvid] = true
					orderedBVIDs = append(orderedBVIDs, bvid)
				}
			}
		}

		if len(orderedBVIDs) == 0 {
			bvidPattern := regexp.MustCompile(`BV[a-zA-Z0-9]{10}`)
			bvids := bvidPattern.FindAllString(html, -1)
			for _, bvid := range bvids {
				if !uniqueBVIDs[bvid] {
					uniqueBVIDs[bvid] = true
					orderedBVIDs = append(orderedBVIDs, bvid)
				}
			}
		}

		log.Printf("从HTML提取到%d个唯一BVID", len(orderedBVIDs))

		videoIndex := 0
		for _, bvid := range orderedBVIDs {
			videoInfo, err := p.getVideoInfoByBVID(bvid)
			if err != nil {
				log.Printf("获取视频信息失败 BVID=%s: %v", bvid, err)
				continue
			}

			videoIndex++
			videoInfo.Page = videoIndex
			info.Videos = append(info.Videos, videoInfo)
			log.Printf("添加视频: BVID=%s, 标题=%s", bvid, videoInfo.Title)
		}
	}

	if len(info.Videos) == 0 {
		return nil, fmt.Errorf("无法从HTML页面提取视频列表 (UpID=%d, SID=%d)", parsedURL.UpID, parsedURL.SID)
	}

	if info.Title == "" {
		info.Title = fmt.Sprintf("合集_%d", parsedURL.SID)
	}
	if info.TotalCount == 0 {
		info.TotalCount = len(info.Videos)
	}

	log.Printf("从HTML最终解析到%d个视频，标题=%s", len(info.Videos), info.Title)
	return info, nil
}

func (p *CollectionParser) getVideoInfoByAID(aid int) (bvid string, aidOut, cid int, err error) {
	apiURL := fmt.Sprintf("https://api.bilibili.com/x/web-interface/view?aid=%d", aid)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", 0, 0, err
	}
	p.setCommonHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, err
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			Bvid string `json:"bvid"`
			Aid  int    `json:"aid"`
			Cid  int    `json:"cid"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", 0, 0, err
	}

	if result.Code != 0 {
		return "", 0, 0, fmt.Errorf("API返回错误码: %d", result.Code)
	}

	return result.Data.Bvid, result.Data.Aid, result.Data.Cid, nil
}

func (p *CollectionParser) getVideoInfoByBVID(bvid string) (*models.CollectionVideoInfo, error) {
	apiURL := fmt.Sprintf("https://api.bilibili.com/x/web-interface/view?bvid=%s", bvid)
	log.Printf("请求视频信息API: %s", apiURL)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	p.setCommonHeaders(req)

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
		Code int `json:"code"`
		Data struct {
			Bvid     string `json:"bvid"`
			Aid      int    `json:"aid"`
			Cid      int    `json:"cid"`
			Title    string `json:"title"`
			Pic      string `json:"pic"`
			Duration int    `json:"duration"`
			Owner    struct {
				Name string `json:"name"`
			} `json:"owner"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API返回错误码: %d", result.Code)
	}

	return &models.CollectionVideoInfo{
		BVID:     result.Data.Bvid,
		AID:      result.Data.Aid,
		CID:      result.Data.Cid,
		Title:    result.Data.Title,
		Author:   result.Data.Owner.Name,
		CoverURL: result.Data.Pic,
		Duration: result.Data.Duration,
	}, nil
}

func (p *CollectionParser) setCommonHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.bilibili.com")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Origin", "https://www.bilibili.com")

	if p.cookies != "" {
		req.Header.Set("Cookie", p.cookies)
	}
}

func (p *CollectionParser) CheckCollectionUpdates(urlStr string, existingBVIDs map[string]bool) ([]*models.CollectionVideoInfo, error) {
	info, err := p.ParseCollection(urlStr)
	if err != nil {
		return nil, err
	}

	var newVideos []*models.CollectionVideoInfo
	for _, v := range info.Videos {
		if !existingBVIDs[v.BVID] {
			newVideos = append(newVideos, v)
		}
	}

	return newVideos, nil
}

func findMatchingBrace(s string, start int) int {
	if start >= len(s) || s[start] != '{' {
		return -1
	}
	
	depth := 0
	inString := false
	escape := false
	
	for i := start; i < len(s); i++ {
		c := s[i]
		
		if escape {
			escape = false
			continue
		}
		
		if c == '\\' && inString {
			escape = true
			continue
		}
		
		if c == '"' {
			inString = !inString
			continue
		}
		
		if inString {
			continue
		}
		
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	
	return -1
}
