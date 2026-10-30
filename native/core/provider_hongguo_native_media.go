package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (downloader *Downloader) resolveHongguoAppMedia(ctx context.Context, videoID string) (providerMedia, error) {
	if !hongguoNumericID.MatchString(videoID) {
		return providerMedia{}, errors.New("红果视频 ID 无效")
	}
	media, signedErr := downloader.resolveHongguoRegisteredMedia(ctx, videoID)
	if signedErr == nil {
		downloader.recordDiagnostic(diagnosticEvent{Event: "hongguo.playback.registered_resolved", Level: "info", Source: sourceHongguo, RequestedQuality: media.Quality})
		return media, nil
	}
	if err := ctx.Err(); err != nil {
		return providerMedia{}, err
	}
	downloader.recordDiagnostic(diagnosticEvent{Event: "hongguo.playback.registered_failed", Level: "warning", Source: sourceHongguo, Message: publicError(signedErr).Error()})
	legacyCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	media, legacyErr := downloader.resolveHongguoLegacyAppMedia(legacyCtx, videoID)
	if legacyErr == nil {
		return media, nil
	}
	return providerMedia{}, fmt.Errorf("匿名设备取流：%v；兼容取流：%w", publicError(signedErr), legacyErr)
}

func (downloader *Downloader) resolveHongguoLegacyAppMedia(ctx context.Context, videoID string) (providerMedia, error) {
	identity, err := downloader.hongguoPlaybackDevice(ctx)
	if err != nil {
		identity, err = downloader.hongguoPlaybackDeviceSession(ctx, true)
		if err != nil {
			return providerMedia{}, err
		}
	}
	ctx = context.WithValue(ctx, hongguoPlaybackIdentityKey{}, identity)
	payload := map[string]any{
		"video_id": videoID, "content_type": 1,
		"biz_param": map[string]any{"need_all_video_definition": true, "video_platform": 3},
	}
	singleCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := downloader.hongguoAppRequest(singleCtx, http.MethodPost, "/novel/player/video_model/v1/", nil, payload)
	cancel()
	if err != nil {
		return downloader.resolveHongguoPreloadMedia(ctx, videoID, err)
	}
	model, err := hongguoVideoModel(nestedMap(result, "data"))
	if err != nil {
		return downloader.resolveHongguoPreloadMedia(ctx, videoID, err)
	}
	media, err := selectHongguoAppMedia(model)
	if err != nil {
		return downloader.resolveHongguoPreloadMedia(ctx, videoID, err)
	}
	return media, nil
}

func hongguoVideoModel(entry map[string]any) (map[string]any, error) {
	model, _ := entry["video_model"].(map[string]any)
	if encoded, ok := entry["video_model"].(string); ok && strings.TrimSpace(encoded) != "" {
		encoded = strings.TrimSpace(encoded)
		if !strings.HasPrefix(encoded, "{") {
			decoded, err := decodeHongguoBase64(encoded)
			if err != nil {
				return nil, errors.New("红果 App 播放信息编码异常")
			}
			encoded = string(decoded)
		}
		decoder := json.NewDecoder(strings.NewReader(encoded))
		decoder.UseNumber()
		if err := decoder.Decode(&model); err != nil || len(model) == 0 {
			return nil, errors.New("红果 App 播放信息格式异常")
		}
	}
	if len(model) == 0 && entry["video_list"] != nil {
		model = map[string]any{
			"video_list": entry["video_list"], "video_duration": entry["video_duration"], "duration": entry["duration"],
		}
	}
	if len(model) == 0 {
		return nil, errors.New("红果 App 未返回本集播放信息")
	}
	return model, nil
}

func (downloader *Downloader) resolveHongguoPreloadMedia(ctx context.Context, videoID string, previous error) (providerMedia, error) {
	payload := map[string]any{
		"biz_param": map[string]any{
			"detail_page_version": 0, "device_level": 3, "disable_digg_stat": false,
			"need_all_video_definition": true, "need_mp4_align": false,
			"use_os_player": false, "use_server_dns": false, "video_platform": 1024,
		},
		"dr_scene":           "preload",
		"mixed_video_id_map": map[string]any{"1004": []string{videoID}},
	}
	result, err := downloader.hongguoAppRequest(ctx, http.MethodPost, "/novel/player/multi_video_model/preload/v1", nil, payload)
	if err != nil {
		return providerMedia{}, fmt.Errorf("单集取流：%v；批量取流：%w", previous, err)
	}
	entry := nestedMap(nestedMap(result, "data"), videoID)
	if len(entry) == 0 {
		return providerMedia{}, errors.New("红果预加载接口未返回所选分集")
	}
	model, err := hongguoVideoModel(entry)
	if err != nil {
		return providerMedia{}, err
	}
	if len(hongguoFallbackRows(model["video_list"])) > 0 {
		return selectHongguoAppMedia(model)
	}
	if fallback, fallbackErr := hongguoFallbackAPI(model); fallbackErr == nil {
		data, err := downloader.fetchHongguoFallbackVideoData(ctx, fallback)
		if err != nil {
			return providerMedia{}, fmt.Errorf("单集取流：%v；批量取流：%w", previous, err)
		}
		media, err := hongguoMediaFromFallbackData(model, data)
		if err != nil {
			return providerMedia{}, fmt.Errorf("单集取流：%v；批量取流：%w", previous, err)
		}
		return media, nil
	}
	media, err := selectHongguoAppMedia(model)
	if err != nil {
		return providerMedia{}, fmt.Errorf("单集取流：%v；批量取流：%w", previous, err)
	}
	return media, nil
}

func selectHongguoAppMedia(model map[string]any) (providerMedia, error) {
	variants := anyList(model["video_list"])
	if rows, ok := model["video_list"].(map[string]any); ok && len(variants) == 0 {
		keys := make([]string, 0, len(rows))
		for key := range rows {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			variants = append(variants, rows[key])
		}
	}
	duration, _ := strconv.ParseFloat(mapString(model, "video_duration", "duration"), 64)
	var keyErr error
	type scoredMedia struct {
		media providerMedia
		score int
	}
	var choices []scoredMedia
	for _, row := range variants {
		variant, _ := row.(map[string]any)
		meta := nestedMap(variant, "video_meta")
		codec := strings.ToLower(firstNonEmpty(mapString(meta, "codec_type"), mapString(variant, "codec_type")))
		if codec == "bytevc2" || strings.Contains(strings.ToLower(mapString(variant, "gear_des_key")), "bytevc2") {
			continue
		}
		addresses := hongguoMediaAddresses(variant)
		if len(addresses) == 0 {
			continue
		}
		media := providerMedia{Referer: "https://novel.snssdk.com/", Duration: time.Duration(duration * float64(time.Second))}
		encryption := nestedMap(variant, "encrypt_info")
		spade := firstNonEmpty(mapString(encryption, "spade_a"), mapString(variant, "spade_a"))
		if spade != "" || encryption["encrypt"] == true || mapString(encryption, "encryption_method") == "cenc-aes-ctr" {
			var err error
			media.CENCKey, err = hongguoAppContentKey(spade)
			if err != nil {
				keyErr = err
				continue
			}
		}
		height, _ := strconv.Atoi(mapString(meta, "vheight"))
		if definition, err := strconv.Atoi(hongguoQualityNumber.FindString(mapString(meta, "definition"))); err == nil && definition > 0 {
			height = definition
		} else if width, _ := strconv.Atoi(mapString(meta, "vwidth")); width > 0 && (height == 0 || width < height) {
			height = width
		}
		media.Quality = height
		quality := height * 10
		if codec == "h264" || codec == "avc1" {
			quality++
		}
		for _, address := range addresses {
			media.URL = address
			choices = append(choices, scoredMedia{media: media, score: quality})
		}
	}
	if len(choices) > 0 {
		sort.SliceStable(choices, func(i, j int) bool { return choices[i].score > choices[j].score })
		selected := choices[0].media
		for _, choice := range choices {
			selected.Variants = append(selected.Variants, choice.media)
		}
		return selected, nil
	}
	if keyErr != nil {
		return providerMedia{}, fmt.Errorf("红果 App 媒体密钥不可用: %w", keyErr)
	}
	return providerMedia{}, errors.New("红果 App 未返回兼容的媒体，已跳过不支持的编码")
}

func hongguoMediaAddresses(info map[string]any) []string {
	var addresses []string
	seen := map[string]bool{}
	var add func(any)
	add = func(value any) {
		switch value := value.(type) {
		case string:
			address := strings.TrimSpace(value)
			if len(address) > 8192 {
				return
			}
			if !isProviderHTTPMediaURL(address) {
				decoded, err := decodeHongguoBase64(address)
				if err != nil {
					return
				}
				address = strings.TrimSpace(string(decoded))
			}
			if isProviderHTTPMediaURL(address) && !seen[address] {
				seen[address] = true
				addresses = append(addresses, address)
			}
		case []any:
			for _, item := range value {
				add(item)
			}
		}
	}
	for _, key := range []string{"main_url", "backup_url", "backup_url_1", "backup_url_2", "backup_urls", "url_list"} {
		add(info[key])
	}
	return addresses
}
