package bilibili

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type WBISigner struct {
	client *http.Client
	imgKey string
	subKey string
}

func NewWBISigner() *WBISigner {
	return &WBISigner{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (w *WBISigner) GetKey() error {
	req, err := http.NewRequest("GET", "https://api.bilibili.com/x/web-interface/nav", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.bilibili.com/")

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Code int `json:"code"`
		Data struct {
			WBI struct {
				ImgURL string `json:"img_url"`
				SubURL string `json:"sub_url"`
			} `json:"wbi"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.Code != 0 {
		return fmt.Errorf("获取WBI密钥失败: code=%d", result.Code)
	}

	w.imgKey = w.extractKeyFromURL(result.Data.WBI.ImgURL)
	w.subKey = w.extractKeyFromURL(result.Data.WBI.SubURL)

	return nil
}

func (w *WBISigner) extractKeyFromURL(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}

	parts := strings.Split(u.Path, "/")
	if len(parts) < 1 {
		return ""
	}

	filename := parts[len(parts)-1]
	extIdx := strings.LastIndex(filename, ".")
	if extIdx != -1 {
		filename = filename[:extIdx]
	}

	return filename
}

func (w *WBISigner) Sign(params map[string]interface{}) (map[string]string, error) {
	if w.imgKey == "" || w.subKey == "" {
		err := w.GetKey()
		if err != nil {
			return nil, err
		}
	}

	if w.imgKey == "" || w.subKey == "" {
		return nil, fmt.Errorf("WBI密钥未初始化")
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	queryParts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := fmt.Sprintf("%v", params[key])
		queryParts = append(queryParts, fmt.Sprintf("%s=%s", key, value))
	}

	sort.Strings(queryParts)

	query := strings.Join(queryParts, "&")
	combined := w.imgKey + query + w.subKey

	hash := md5.Sum([]byte(combined))
	hashBytes := hash[:]

	w_rid := hex.EncodeToString(hashBytes)

	result := map[string]string{
		"w_rid": w_rid,
		"wts":   fmt.Sprintf("%d", time.Now().Unix()),
	}

	return result, nil
}
