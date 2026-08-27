package bilibili

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Downloader struct {
	client  *http.Client
	cookies string
}

func NewDownloader() *Downloader {
	return &Downloader{
		client: &http.Client{
			Timeout: 600 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

func (d *Downloader) SetCookies(cookies string) {
	d.cookies = cookies
}

// Download 下载单个流（视频或音频）
func (d *Downloader) Download(url, outputPath string, progressFunc func(int64, int64)) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	// 根据URL设置请求头
	if strings.Contains(url, "bilibili.com") || strings.Contains(url, "hdslb.com") || strings.Contains(url, "bilivideo.com") {
		d.setBilibiliHeaders(req)
	} else if strings.Contains(url, "douyin.com") || strings.Contains(url, "bytecdn.cn") || strings.Contains(url, "byteimg.com") {
		d.setDouyinHeaders(req)
	} else {
		d.setHeaders(req)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// 检查Content-Type，拒绝HTML错误页面
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && contentType != "application/octet-stream" && contentType != "video/mp4" && contentType != "audio/mp4" {
		// 读取前100字节检查是否是HTML
		buf := make([]byte, 100)
		n, _ := io.ReadFull(resp.Body, buf)
		if n > 0 && (strings.HasPrefix(string(buf[:n]), "<!DOCTYPE") || strings.HasPrefix(string(buf[:n]), "<html")) {
			return fmt.Errorf("服务器返回HTML而非视频流")
		}
		// 如果不是HTML，继续下载（需要把已读取的部分写入）
		outFile, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		defer outFile.Close()
		outFile.Write(buf[:n])
		written := int64(n)
		totalSize := resp.ContentLength
		if progressFunc != nil {
			progressFunc(written, totalSize)
		}
		buf2 := make([]byte, 32*1024)
		for {
			nr, err := resp.Body.Read(buf2)
			if nr > 0 {
				nw, ew := outFile.Write(buf2[:nr])
				if ew != nil {
					return ew
				}
				written += int64(nw)
				if progressFunc != nil {
					progressFunc(written, totalSize)
				}
			}
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
		}
		return nil
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	totalSize := resp.ContentLength
	var written int64
	buf := make([]byte, 32*1024)

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			nw, ew := outFile.Write(buf[:n])
			if ew != nil {
				return ew
			}
			written += int64(nw)
			if progressFunc != nil {
				progressFunc(written, totalSize)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	return nil
}

// DownloadWithMerge 下载DASH格式视频并合并
func (d *Downloader) DownloadWithMerge(videoURL, audioURL, outputPath string, progressFunc func(int64, int64)) error {
	if audioURL == "" {
		// 非DASH格式，直接下载
		return d.Download(videoURL, outputPath, progressFunc)
	}

	// 生成临时文件名
	videoTemp := fmt.Sprintf("temp_video_%d.m4s", time.Now().UnixNano())
	audioTemp := fmt.Sprintf("temp_audio_%d.m4s", time.Now().UnixNano())
	defer os.Remove(videoTemp)
	defer os.Remove(audioTemp)

	// 下载视频流（带重试）
	var videoErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}
		videoErr = d.Download(videoURL, videoTemp, progressFunc)
		if videoErr == nil {
			break
		}
	}
	if videoErr != nil {
		return fmt.Errorf("下载视频流失败(重试3次): %w", videoErr)
	}

	// 下载音频流（带重试）
	var audioErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}
		audioErr = d.Download(audioURL, audioTemp, progressFunc)
		if audioErr == nil {
			break
		}
	}
	if audioErr != nil {
		return fmt.Errorf("下载音频流失败(重试3次): %w", audioErr)
	}

	// 清理M4S头部
	videoClean := videoTemp + ".clean.mp4"
	audioClean := audioTemp + ".clean.m4a"
	defer os.Remove(videoClean)
	defer os.Remove(audioClean)

	if err := removeM4SHeader(videoTemp, videoClean); err != nil {
		return fmt.Errorf("清理视频M4S头部失败: %w", err)
	}

	if err := removeM4SHeader(audioTemp, audioClean); err != nil {
		return fmt.Errorf("清理音频M4S头部失败: %w", err)
	}

	// 合并音视频
	return MergeMP4(videoClean, audioClean, outputPath)
}

// DownloadWithMergeURLs 使用候选URL列表下载DASH格式视频并合并
func (d *Downloader) DownloadWithMergeURLs(videoURLs, audioURLs []string, outputPath string, progressFunc func(int64, int64)) error {
	if len(videoURLs) == 0 {
		return fmt.Errorf("没有可用的视频流URL")
	}
	if len(audioURLs) == 0 {
		// 非DASH格式，只下载视频
		return d.Download(videoURLs[0], outputPath, progressFunc)
	}

	// 生成临时文件名
	videoTemp := fmt.Sprintf("temp_video_%d.m4s", time.Now().UnixNano())
	audioTemp := fmt.Sprintf("temp_audio_%d.m4s", time.Now().UnixNano())
	defer os.Remove(videoTemp)
	defer os.Remove(audioTemp)

	// 下载视频流（带重试和CDN切换）
	videoErr := d.downloadStreamWithFallback(videoURLs, videoTemp, "视频", progressFunc)
	if videoErr != nil {
		return fmt.Errorf("下载视频流失败: %w", videoErr)
	}

	// 下载音频流（带重试和CDN切换）
	audioErr := d.downloadStreamWithFallback(audioURLs, audioTemp, "音频", progressFunc)
	if audioErr != nil {
		return fmt.Errorf("下载音频流失败: %w", audioErr)
	}

	// 清理M4S头部
	videoClean := videoTemp + ".clean.mp4"
	audioClean := audioTemp + ".clean.m4a"
	defer os.Remove(videoClean)
	defer os.Remove(audioClean)

	if err := removeM4SHeader(videoTemp, videoClean); err != nil {
		return fmt.Errorf("清理视频M4S头部失败: %w", err)
	}

	if err := removeM4SHeader(audioTemp, audioClean); err != nil {
		return fmt.Errorf("清理音频M4S头部失败: %w", err)
	}

	// 合并音视频
	return MergeMP4(videoClean, audioClean, outputPath)
}

// downloadStreamWithFallback 使用候选URL列表下载流，支持重试和CDN切换
func (d *Downloader) downloadStreamWithFallback(candidates []string, outputPath string, streamType string, progressFunc func(int64, int64)) error {
	if len(candidates) == 0 {
		return fmt.Errorf("没有可用的%s流URL", streamType)
	}

	for i, candidateURL := range candidates {
		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 || i > 0 {
				time.Sleep(time.Duration(attempt+1) * time.Second)
			}
			err := d.Download(candidateURL, outputPath, progressFunc)
			if err == nil {
				return nil
			}
			// 如果是连接拒绝错误，切换到下一个候选URL
			if strings.Contains(err.Error(), "connectex") || strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "actively refused") {
				if i < len(candidates)-1 {
					log.Printf("[B站-CDN] %s流当前节点失败(%v)，切换到备用节点", streamType, err)
					break // 跳出重试循环，尝试下一个候选URL
				}
			}
			if attempt == 2 && i == len(candidates)-1 {
				return fmt.Errorf("下载%s流失败(所有候选节点均失败): %w", streamType, err)
			}
		}
	}
	return fmt.Errorf("下载%s流失败: 所有候选URL均不可用", streamType)
}

// removeM4SHeader 剥离B站M4S文件的8字节私有头部
func removeM4SHeader(inputPath, outputPath string) error {
	input, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer input.Close()

	// 读取前8字节检查是否需要剥离
	header := make([]byte, 8)
	n, err := io.ReadFull(input, header)
	if err != nil {
		return err
	}

	// 检查前8字节是否全为0
	allZero := true
	for i := 0; i < n; i++ {
		if header[i] != 0 {
			allZero = false
			break
		}
	}

	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()

	// 如果前8字节全为0，跳过这8字节
	if allZero && n == 8 {
		// 跳过8字节头部，复制剩余内容
		_, err = io.Copy(output, input)
		return err
	}

	// 否则写入已读取的头部，然后复制剩余内容
	output.Write(header[:n])
	_, err = io.Copy(output, input)
	return err
}

func (d *Downloader) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	if d.cookies != "" {
		req.Header.Set("Cookie", d.cookies)
	}
}

func (d *Downloader) setBilibiliHeaders(req *http.Request) {
	d.setHeaders(req)
	req.Header.Set("Referer", "https://www.bilibili.com")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Origin", "https://www.bilibili.com")
	req.Header.Set("Sec-Fetch-Dest", "video")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
}

func (d *Downloader) setDouyinHeaders(req *http.Request) {
	d.setHeaders(req)
	req.Header.Set("Referer", "https://www.douyin.com/")
}
