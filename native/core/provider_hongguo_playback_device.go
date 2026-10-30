// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const hongguoPlaybackRegisterURL = "https://log5-applog-lf.fqnovel.com/service/2/device_register/?tt_data=a&cronet_version=8d40f833_2026-03-03&ttnet_version=4.2.243.31-douyin&use_store_region_cookie=1"

const hongguoRegisteredMediaUA = "com.phoenix.read/71532 (Linux; U; Android 9; SM-N9860; Build/PQ3A.190705.10241111;tt-ok/3.12.13.20)"

type hongguoPlaybackIdentity struct {
	deviceID  string
	installID string
	cdid      string
}

type hongguoPlaybackIdentityKey struct{}

type hongguoPlaybackDeviceCall struct {
	done     chan struct{}
	identity hongguoPlaybackIdentity
	err      error
}

func newHongguoPlaybackUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = raw[6]&15 | 64
	raw[8] = raw[8]&63 | 128
	value := hex.EncodeToString(raw[:])
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:], nil
}

func (downloader *Downloader) hongguoPlaybackDevice(ctx context.Context) (hongguoPlaybackIdentity, error) {
	return downloader.hongguoPlaybackDeviceSession(ctx, false)
}

func (downloader *Downloader) hongguoPlaybackDeviceSession(ctx context.Context, compatibility bool) (hongguoPlaybackIdentity, error) {
	if err := ctx.Err(); err != nil {
		return hongguoPlaybackIdentity{}, err
	}
	client := downloader.hongguoClient()
	client.mu.Lock()
	call := client.playbackDevice
	if compatibility {
		call = client.compatPlaybackDevice
	}
	if call == nil {
		call = &hongguoPlaybackDeviceCall{done: make(chan struct{})}
		if compatibility {
			client.compatPlaybackDevice = call
		} else {
			client.playbackDevice = call
		}
		go func() {
			registrationCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			var identity hongguoPlaybackIdentity
			var err error
			if compatibility {
				identity.deviceID, identity.installID, err = downloader.registerHongguoDevice(registrationCtx)
				if err == nil && (strings.Trim(identity.deviceID, "0") == "" || strings.Trim(identity.installID, "0") == "") {
					err = errors.New("兼容播放设备号无效")
				}
			} else {
				identity, err = downloader.registerHongguoPlaybackDevice(registrationCtx)
			}
			if err != nil {
				call.err = errors.New("匿名播放设备注册失败，请检查网络或重启应用后重试")
			} else {
				call.identity = identity
			}
			close(call.done)
		}()
	}
	client.mu.Unlock()
	select {
	case <-ctx.Done():
		return hongguoPlaybackIdentity{}, ctx.Err()
	case <-call.done:
		return call.identity, call.err
	}
}

func hongguoRegisteredID(result map[string]any, name string) string {
	value := result[name+"_str"]
	if value == nil || value == "" {
		value = result[name]
	}
	var text string
	switch typed := value.(type) {
	case string:
		text = typed
	case json.Number:
		text = typed.String()
	default:
		return ""
	}
	if len(text) == 0 || len(text) > 30 || !hongguoNumericID.MatchString(text) || strings.Trim(text, "0") == "" {
		return ""
	}
	return text
}

func (downloader *Downloader) registerHongguoPlaybackDevice(ctx context.Context) (hongguoPlaybackIdentity, error) {
	cdid, err := newHongguoPlaybackUUID()
	if err != nil {
		return hongguoPlaybackIdentity{}, err
	}
	clientUDID, err := newHongguoPlaybackUUID()
	if err != nil {
		return hongguoPlaybackIdentity{}, err
	}
	requestID, err := newHongguoPlaybackUUID()
	if err != nil {
		return hongguoPlaybackIdentity{}, err
	}
	openUDID := randomHex(8)
	if len(openUDID) != 16 {
		return hongguoPlaybackIdentity{}, errors.New("匿名设备参数生成失败")
	}
	now := time.Now().UnixMilli()
	header := map[string]any{
		"display_name": "红果免费短剧", "update_version_code": 73932, "manifest_version_code": 73932,
		"app_version_minor": "7.3.9.32", "aid": 8662, "channel": "update_64", "package": "com.phoenix.read",
		"app_version": "7.3.9.32", "version_code": 73932, "sdk_version": "3.7.0-rc.34-fanqie-xiaoshuo",
		"sdk_target_version": 29, "git_hash": "3a88f4e", "os": "Android", "os_version": "16", "os_api": 36,
		"device_model": "25053RT47C", "device_brand": "Redmi", "device_manufacturer": "Redmi",
		"cpu_abi": "arm64-v8a", "release_build": "b5e187f", "density_dpi": 520, "display_density": "mdpi",
		"resolution": "1280x2772", "language": "zh", "timezone": 8, "access": "wifi", "not_request_sender": 0,
		"carrier": "CHINA MOBILE", "mcc_mnc": "46000", "rom": "eng.root.20231116.160340",
		"rom_version": "W528JS release-keys", "cdid": cdid,
		"sig_hash": "56a962410c494bbaf0b58dba20cae56f", "openudid": openUDID,
		"clientudid": clientUDID, "ipv6_list": []string{}, "region": "CN", "tz_name": "Asia/Shanghai",
		"tz_offset": 28800, "sim_region": "cn", "sim_serial_number": []string{}, "oaid_may_support": false,
		"req_id": requestID, "device_platform": "android",
		"custom":                map[string]any{"account_region_src": "fallback", "account_region": "cn", "host_bit": 64, "dragon_device_type": "phone"},
		"pre_installed_channel": "", "apk_first_install_time": now, "is_system_app": 0, "sdk_flavor": "china", "guest_mode": 0,
	}
	payload, err := json.Marshal(map[string]any{"magic_tag": "ss_app_log", "header": header, "_gen_time": now})
	if err != nil {
		return hongguoPlaybackIdentity{}, err
	}
	body, err := hongguoEncryptDeviceRegistration(payload)
	if err != nil {
		return hongguoPlaybackIdentity{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, hongguoPlaybackRegisterURL, bytes.NewReader(body))
	if err != nil {
		return hongguoPlaybackIdentity{}, err
	}
	request.Header.Set("User-Agent", "com.phoenix.read/72232 (Linux; U; Android 16; zh_CN; 25053RT47C; Build/BP2A.250605.031.A3)")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/octet-stream;tt-data=a")
	request.Header.Set("Log-Encode-Type", "gzip")
	request.Header.Set("X-SS-DP", "8662")
	request.Header.Set("X-SS-Req-Ticket", strconv.FormatInt(now, 10))
	request.Header.Set("X-Vc-Bdturing-Sdk-Version", "4.0.3.cn")
	request.Header.Set("Lc", "101")
	request.Header.Set("Sdk-Version", "2")
	request.Header.Set("Passport-Sdk-Version", "5051452")
	request.Header.Set("X-Neptune", "-8|50:51:59:30:40:47:49:39")
	request.Header.Set("X-SS-STUB", hgMd5HexUpper(body))
	result, err := downloader.hongguoPlaybackJSON(request, nil)
	if err != nil {
		return hongguoPlaybackIdentity{}, err
	}
	deviceID, installID := hongguoRegisteredID(result, "device_id"), hongguoRegisteredID(result, "install_id")
	if deviceID == "" || installID == "" {
		return hongguoPlaybackIdentity{}, errors.New("匿名设备注册未返回有效设备号")
	}
	return hongguoPlaybackIdentity{deviceID: deviceID, installID: installID, cdid: cdid}, nil
}

func hongguoEncryptDeviceRegistration(payload []byte) ([]byte, error) {
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	if _, err := compressed.Write(payload); err != nil {
		return nil, err
	}
	if err := compressed.Close(); err != nil {
		return nil, err
	}
	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, err
	}
	first := sha512.Sum512(seed[:])
	material := append(append([]byte(nil), first[:]...), hongguoSpadeConstants[:]...)
	derived := sha512.Sum512(material)
	block, err := aes.NewCipher(derived[:16])
	if err != nil {
		return nil, err
	}
	digest := sha512.Sum512(buffer.Bytes())
	plain := append(append([]byte(nil), digest[:]...), buffer.Bytes()...)
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(plain, bytes.Repeat([]byte{byte(padding)}, padding)...)
	cipher.NewCBCEncrypter(block, derived[16:32]).CryptBlocks(plain, plain)
	body := append([]byte{0x74, 0x63, 0x05, 0x10, 0x00, 0x00}, seed[:]...)
	return append(body, plain...), nil
}
