// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	hongguoSignedMediaURL = "https://api5-normal-sinfonlineb.fqnovel.com/novel/player/multi_video_model/v1/"
	hongguoSignedMediaUA  = "com.phoenix.read/71332 (Linux; U; Android 16; zh_CN; 25053RT47C; Build/BP2A.250605.031.A3; Cronet/TTNetVersion:04657795 2026-01-23 QuicVersion:c67e9834 2025-09-08)"
)

var errHongguoPlaybackEmpty = errors.New("红果播放接口空响应，可能与设备鉴权或签名有关")

func (downloader *Downloader) resolveHongguoRegisteredMedia(ctx context.Context, videoID string) (providerMedia, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	identity, err := downloader.hongguoPlaybackDevice(ctx)
	if err != nil {
		return providerMedia{}, err
	}
	media, err := downloader.requestHongguoAppMedia(ctx, videoID, identity, false)
	if errors.Is(err, errHongguoPlaybackEmpty) && ctx.Err() == nil {
		return downloader.requestHongguoAppMedia(ctx, videoID, identity, true)
	}
	return media, err
}

func hongguoSignedMediaQuery(deviceID, installID string, timestamp, ticket int64) string {
	fields := [][2]string{
		{"ac", "wifi"}, {"channel", "update_64"}, {"aid", "8662"}, {"app_name", "novelread"},
		{"version_code", "71332"}, {"version_name", "7.1.3.32"}, {"device_platform", "android"},
		{"os", "android"}, {"ssmix", "a"}, {"device_type", "25053RT47C"}, {"device_brand", "Redmi"},
		{"language", "zh"}, {"os_api", "36"}, {"os_version", "16"}, {"manifest_version_code", "71332"},
		{"resolution", "1280*2772"}, {"dpi", "520"}, {"update_version_code", "71332"},
		{"host_abi", "arm64-v8a"}, {"dragon_device_type", "phone"}, {"pv_player", "71332"},
		{"compliance_status", "0"}, {"need_personal_recommend", "1"}, {"player_so_load", "1"},
		{"is_android_pad_screen", "0"}, {"iid", installID}, {"device_id", deviceID},
		{"ts", strconv.FormatInt(timestamp, 10)}, {"_rticket", strconv.FormatInt(ticket, 10)},
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, url.QueryEscape(field[0])+"="+url.QueryEscape(field[1]))
	}
	return strings.Join(parts, "&")
}

func (downloader *Downloader) requestHongguoAppMedia(ctx context.Context, videoID string, identity hongguoPlaybackIdentity, signed bool) (providerMedia, error) {
	body, err := json.Marshal(map[string]any{
		"biz_param": map[string]any{
			"detail_page_version": 0, "device_level": 3, "disable_digg_stat": false,
			"need_all_video_definition": true, "need_mp4_align": false,
			"use_os_player": false, "use_server_dns": false, "video_platform": 1024,
		},
		"mixed_video_id_map": map[string]any{"1004": []string{videoID}},
	})
	if err != nil {
		return providerMedia{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, hongguoSignedMediaURL, bytes.NewReader(body))
	if err != nil {
		return providerMedia{}, err
	}
	agent := hongguoRegisteredMediaUA
	if signed {
		agent = hongguoSignedMediaUA
	}
	request.Header.Set("User-Agent", agent)
	request.Header.Set("Accept", "application/json; charset=utf-8,application/x-protobuf")
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	if signed {
		request.Header.Set("X-XS-From-Web", "0")
		request.Header.Set("X-Tt-Request-Tag", "t=0;n=0")
		request.Header.Set("Sdk-Version", "2")
		request.Header.Set("Passport-Sdk-Version", "50561")
		request.Header.Set("X-Vc-Bdturing-Sdk-Version", "3.7.2.cn")
		request.Header.Set("X-Tt-Dt", "")
	}
	result, err := downloader.hongguoPlaybackJSON(request, func(prepared *http.Request) error {
		now := time.Now()
		if !signed {
			query, err := url.ParseQuery(hongguoSignedMediaQuery(identity.deviceID, identity.installID, now.Unix(), now.UnixMilli()))
			if err != nil {
				return err
			}
			query.Del("ts")
			query.Set("cdid", identity.cdid)
			prepared.URL.RawQuery = query.Encode()
			return nil
		}
		for offset := int64(0); offset < 32; offset++ {
			ticket := now.UnixMilli() + offset
			query := hongguoSignedMediaQuery(identity.deviceID, identity.installID, now.Unix(), ticket) + "&cdid=" + url.QueryEscape(identity.cdid)
			if hgBranchOf(query, body, uint32(now.Unix())) == 1 {
				continue
			}
			prepared.URL.RawQuery = query
			prepared.Header.Set("X-SS-Req-Ticket", strconv.FormatInt(ticket, 10))
			prepared.Header.Set("X-SS-STUB", hgMd5HexUpper(body))
			for name, value := range hongguoPureSign(prepared.URL.String(), body, uint32(now.Unix())) {
				prepared.Header.Set(name, value)
			}
			return nil
		}
		return errors.New("红果播放签名参数暂不可用")
	})
	if err != nil {
		return providerMedia{}, err
	}
	entry := nestedMap(nestedMap(result, "data"), videoID)
	if len(entry) == 0 {
		return providerMedia{}, errors.New("红果批量接口未返回所选分集")
	}
	model, err := hongguoVideoModel(entry)
	if err != nil {
		return providerMedia{}, err
	}
	if len(hongguoFallbackRows(model["video_list"])) > 0 {
		return hongguoInlineMedia(model, agent)
	}
	fallback, err := hongguoFallbackAPI(model)
	if err != nil {
		return providerMedia{}, err
	}
	data, err := downloader.fetchHongguoFallbackVideoData(ctx, fallback)
	if err != nil {
		return providerMedia{}, err
	}
	return hongguoMediaFromFallbackData(model, data)
}

func hongguoInlineMedia(model map[string]any, agent string) (providerMedia, error) {
	var rows []any
	for _, item := range hongguoFallbackRows(model["video_list"]) {
		encryption := nestedMap(item, "encrypt_info")
		spade := firstNonEmpty(mapString(item, "spade_a"), mapString(encryption, "spade_a"))
		flag := mapString(encryption, "encrypt")
		if spade == "" && flag != "false" && flag != "0" {
			continue
		}
		rows = append(rows, item)
	}
	if len(rows) == 0 {
		return providerMedia{}, errors.New("本集片源缺少有效解密信息")
	}
	inline := make(map[string]any, len(model))
	for key, value := range model {
		inline[key] = value
	}
	inline["video_list"] = rows
	media, err := hongguoMediaFromFallbackData(inline, inline)
	if err != nil {
		return providerMedia{}, err
	}
	media.credentials = &providerMediaCredentials{userAgent: agent}
	for index := range media.Variants {
		media.Variants[index].credentials = &providerMediaCredentials{userAgent: agent}
	}
	return media, nil
}

func (downloader *Downloader) hongguoPlaybackJSON(request *http.Request, prepare func(*http.Request) error) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()
	request = request.Clone(ctx)
	if downloader.limiter != nil {
		release, err := downloader.limiter.acquire(ctx, request)
		if err != nil {
			return nil, err
		}
		defer release()
	}
	if prepare != nil {
		if err := prepare(request); err != nil {
			return nil, err
		}
	}
	client := *downloader.client
	client.Jar = nil
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if request.Method != http.MethodGet || len(via) >= 3 || !hongguoAllowedFallbackURL(next.URL) {
			return errors.New("红果播放接口跳转无效")
		}
		if previousRedirect != nil {
			return previousRedirect(next, via)
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if downloader.limiter != nil {
		downloader.limiter.observe(request, response)
	}
	observeSourceResponse(ctx, response)
	content, err := io.ReadAll(io.LimitReader(response.Body, providerMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, downloader.catalogResponseError(request, response, content)
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, errHongguoPlaybackEmpty
	}
	if len(content) > providerMaxBodyBytes {
		return nil, errors.New("红果播放接口响应过大")
	}
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if decoder.Decode(&result) != nil || len(result) == 0 {
		return nil, errors.New("红果播放接口返回格式异常")
	}
	if err := hongguoPlaybackResponseError(result); err != nil {
		return nil, err
	}
	return result, nil
}

func hongguoPlaybackResponseError(result map[string]any) error {
	for _, code := range []string{mapString(result, "code"), mapString(result, "Code"), mapString(result, "status_code"), mapString(nestedMap(result, "BaseResp"), "StatusCode")} {
		if code != "" && code != "0" {
			return fmt.Errorf("红果播放接口暂不可用（%s）", truncate(code, 20))
		}
	}
	return nil
}
