// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func hongguoFallbackAPI(model map[string]any) (string, error) {
	value := model["fallback_api"]
	for depth := 0; depth < 5; depth++ {
		switch typed := value.(type) {
		case string:
			text := strings.TrimSpace(typed)
			if len(text) == 0 || len(text) > 8192 {
				return "", errors.New("红果取流响应缺少有效 fallback_api")
			}
			if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
				if err := json.Unmarshal([]byte(text), &value); err != nil {
					return "", errors.New("红果 fallback_api 格式异常")
				}
				continue
			}
			return hongguoCleanPlaybackURL(text), nil
		case map[string]any:
			value = typed["fallback_api"]
		case []any:
			if len(typed) == 0 {
				return "", errors.New("红果取流响应缺少 fallback_api")
			}
			value = typed[0]
		default:
			return "", errors.New("红果取流响应缺少 fallback_api")
		}
	}
	return "", errors.New("红果 fallback_api 嵌套过深")
}

func (downloader *Downloader) fetchHongguoFallbackVideoData(ctx context.Context, address string) (map[string]any, error) {
	parsed, err := url.Parse(address)
	if err != nil || !hongguoAllowedFallbackURL(parsed) {
		return nil, errors.New("红果备用播放地址无效")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", hongguoSignedMediaUA)
	request.Header.Set("Accept", "application/json")
	result, err := downloader.hongguoPlaybackJSON(request, nil)
	if err != nil {
		return nil, err
	}
	videoInfo := nestedMap(result, "video_info")
	if err := hongguoPlaybackResponseError(videoInfo); err != nil {
		return nil, err
	}
	videoData := nestedMap(videoInfo, "data")
	if len(videoData) == 0 {
		return nil, errors.New("红果备用播放接口缺少本集 video_info")
	}
	return videoData, nil
}

func hongguoFallbackRows(value any) []map[string]any {
	var rows []map[string]any
	add := func(value any) {
		if item, ok := value.(map[string]any); ok && len(item) > 0 {
			rows = append(rows, item)
		}
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			add(typed[key])
		}
	case []any:
		for _, item := range typed {
			add(item)
		}
	}
	return rows
}

func hongguoFallbackAddresses(item map[string]any) []string {
	var addresses []string
	var add func(any)
	add = func(value any) {
		switch typed := value.(type) {
		case string:
			if text := strings.TrimSpace(typed); text != "" && len(text) <= 8192 {
				addresses = append(addresses, text)
			}
		case []any:
			for _, value := range typed {
				if text, ok := value.(string); ok {
					add(text)
				}
			}
		}
	}
	for _, key := range []string{"main_url", "backup_url", "backup_url_1", "backup_url_2", "backup_url_list", "backup_urls", "url_list"} {
		add(item[key])
	}
	return addresses
}

func hongguoFallbackQuality(item, meta map[string]any) int {
	for _, dimensions := range [][2]string{
		{mapString(item, "width", "vwidth"), mapString(item, "height", "vheight")},
		{mapString(meta, "width", "vwidth"), mapString(meta, "height", "vheight")},
	} {
		width, _ := strconv.Atoi(dimensions[0])
		height, _ := strconv.Atoi(dimensions[1])
		if width > 0 && height > 0 {
			return min(width, height)
		}
	}
	for _, value := range []string{mapString(item, "quality_desc", "definition", "vheight", "height"), mapString(meta, "definition", "vheight", "height")} {
		if quality, _ := strconv.Atoi(hongguoQualityNumber.FindString(value)); quality > 0 {
			return quality
		}
	}
	return 0
}

func hongguoMediaFromFallbackData(model, videoData map[string]any) (providerMedia, error) {
	var keySeed []byte
	if mapString(videoData, "key_seed") != "" {
		var err error
		keySeed, err = hongguoSpadeKeySeed(videoData)
		if err != nil {
			return providerMedia{}, err
		}
	}
	rows := hongguoFallbackRows(videoData["video_list"])
	if len(rows) == 0 {
		return providerMedia{}, errors.New("红果备用播放接口无可播档位")
	}
	duration, _ := strconv.ParseFloat(firstNonEmpty(mapString(videoData, "video_duration", "duration"), mapString(model, "video_duration", "duration")), 64)
	type candidate struct {
		media     providerMedia
		codecRank int
	}
	var candidates []candidate
	seen := map[string]bool{}
	var keyErr error
	for _, item := range rows {
		meta := nestedMap(item, "video_meta")
		codec := strings.ToLower(firstNonEmpty(mapString(item, "codec_type"), mapString(meta, "codec_type")))
		codecRank := 0
		switch codec {
		case "h264", "avc", "avc1":
		case "hevc", "h265", "hvc1", "hev1", "bytevc1":
			codecRank = 1
		default:
			continue
		}
		var contentKey []byte
		encryption := nestedMap(item, "encrypt_info")
		spade := firstNonEmpty(mapString(item, "spade_a"), mapString(encryption, "spade_a"))
		if spade != "" {
			var err error
			contentKey, err = hongguoAppContentKey(spade)
			if err != nil {
				keyErr = err
				continue
			}
		} else if encryption["encrypt"] == true || mapString(encryption, "encryption_method") == "cenc-aes-ctr" {
			keyErr = errors.New("红果备用播放媒体缺少密钥")
			continue
		}
		for _, raw := range hongguoFallbackAddresses(item) {
			address := raw
			if len(keySeed) > 0 {
				var err error
				address, err = decryptHongguoSpadeURL(raw, keySeed)
				if err != nil {
					continue
				}
			} else if !isProviderHTTPMediaURL(address) {
				decoded, err := decodeHongguoBase64(address)
				if err != nil {
					continue
				}
				address = string(decoded)
			}
			address = hongguoCleanPlaybackURL(address)
			if !hongguoAllowedMediaURL(address) {
				continue
			}
			identity := address + ":" + fmt.Sprintf("%x", contentKey)
			if seen[identity] {
				continue
			}
			seen[identity] = true
			media := providerMedia{URL: address, CENCKey: contentKey, Duration: time.Duration(duration * float64(time.Second)), Quality: hongguoFallbackQuality(item, meta), credentials: &providerMediaCredentials{userAgent: hongguoSignedMediaUA}}
			candidates = append(candidates, candidate{media: media, codecRank: codecRank})
		}
	}
	if len(candidates) == 0 {
		if keyErr != nil {
			return providerMedia{}, fmt.Errorf("红果 App 媒体密钥不可用: %w", keyErr)
		}
		return providerMedia{}, errors.New("红果 App 未返回地址有效的 H.264 / HEVC 片源")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].codecRank != candidates[j].codecRank {
			return candidates[i].codecRank < candidates[j].codecRank
		}
		return candidates[i].media.Quality > candidates[j].media.Quality
	})
	selected := candidates[0].media
	for _, item := range candidates {
		selected.Variants = append(selected.Variants, item.media)
	}
	return selected, nil
}

func hongguoCleanPlaybackURL(value string) string {
	return strings.TrimSpace(strings.NewReplacer(`\/`, `/`, `\u0026`, `&`, `&amp;`, `&`).Replace(value))
}

func hongguoAllowedPlaybackURL(parsed *url.URL, domains []string) bool {
	if parsed == nil || parsed.Scheme != "https" || parsed.User != nil || (parsed.Port() != "" && parsed.Port() != "443") {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func hongguoAllowedFallbackURL(parsed *url.URL) bool {
	return hongguoAllowedPlaybackURL(parsed, []string{"fqnovel.com", "snssdk.com"})
}

func hongguoAllowedMediaURL(address string) bool {
	parsed, err := url.Parse(address)
	return err == nil && hongguoAllowedPlaybackURL(parsed, []string{"qznovelvod.com", "fqnovelvod.com", "douyinvod.com"})
}
