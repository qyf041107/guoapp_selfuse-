// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
)

// 红果取流失败按「能否通过重新签名 / 更换匿名设备 / 更换网络出口恢复」
// 分类。分类只影响重试策略；用户看到的文案仍由 hongguoPlaybackUserMessage
// 统一生成，避免把内部地址、签名参数或设备号带到界面。
var (
	errHongguoPlaybackEmpty          = errors.New("红果播放接口空响应，可能与设备鉴权或签名有关")
	errHongguoPlaybackAuth           = errors.New("红果播放接口鉴权失败")
	errHongguoPlaybackMalformed      = errors.New("红果播放接口返回格式异常")
	errHongguoPlaybackMissingEpisode = errors.New("红果批量接口未返回所选分集")
	errHongguoPlaybackPayloadEmpty   = errors.New("红果播放接口未返回本集播放信息")
	errHongguoPlaybackData           = errors.New("红果播放片源数据不可用")
	errHongguoPlaybackRegistration   = errors.New("匿名播放设备注册失败")
)

// hongguoPlaybackClassError 保留原有的用户可见文案，同时允许 errors.Is
// 判定失败类别。
type hongguoPlaybackClassError struct {
	class   error
	message string
}

func (err *hongguoPlaybackClassError) Error() string { return err.message }
func (err *hongguoPlaybackClassError) Unwrap() error { return err.class }

func hongguoPlaybackClassify(class error, message string) error {
	return &hongguoPlaybackClassError{class: class, message: message}
}

// hongguoPlaybackCodeError 对应接口 HTTP 200 但业务 code 非 0 的情况。
// 这类失败通常与设备身份或签名有关，可以重新签名并更换匿名设备重试。
type hongguoPlaybackCodeError struct {
	code string
}

func (err *hongguoPlaybackCodeError) Error() string {
	return fmt.Sprintf("红果播放接口暂不可用（%s）", truncate(err.code, 20))
}

func (err *hongguoPlaybackCodeError) Is(target error) bool {
	return target == errHongguoPlaybackAuth
}

// hongguoPlaybackRegistrationError 不暴露底层注册地址或响应内容；底层
// 原因通过 Unwrap 保留给诊断日志。
type hongguoPlaybackRegistrationError struct {
	cause error
}

func (err *hongguoPlaybackRegistrationError) Error() string {
	return "匿名播放设备注册失败，请检查网络或切换网络后重试"
}

func (err *hongguoPlaybackRegistrationError) Is(target error) bool {
	return target == errHongguoPlaybackRegistration
}

func (err *hongguoPlaybackRegistrationError) Unwrap() error { return err.cause }

// hongguoPlaybackAuthRetryable 返回 true 时，表示换用六签名或重新注册匿名
// 设备有可能恢复。限流退避（403/429）和网络错误不在此列：前者换设备号
// 没有帮助，后者应交给网络出口切换。
func hongguoPlaybackAuthRetryable(err error) bool {
	if err == nil {
		return false
	}
	var backoff *requestBackoff
	if errors.As(err, &backoff) {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return errors.Is(err, errHongguoPlaybackEmpty) ||
		errors.Is(err, errHongguoPlaybackAuth) ||
		errors.Is(err, errHongguoPlaybackMalformed) ||
		errors.Is(err, errHongguoPlaybackPayloadEmpty)
}

// hongguoPlaybackEgressBlocked 判断失败是否可能由当前网络出口引起。只有
// 这类错误才值得在 Android 上临时切到物理网络重试。浏览器验证页对出口
// IP 的判定不会因为换到物理网络而改善，因此不包含在信号里。
func hongguoPlaybackEgressBlocked(err error) bool {
	if err == nil {
		return false
	}
	var backoff *requestBackoff
	if errors.As(err, &backoff) {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return false
	}
	var certificateErr *tls.CertificateVerificationError
	if errors.As(err, &certificateErr) {
		return false
	}
	text := strings.ToLower(publicError(err).Error())
	for _, signal := range []string{
		"connection reset", "connection refused", "connection aborted", "broken pipe",
		"unexpected eof", "no route to host", "network is unreachable",
		"no application protocol", "tls handshake", "handshake failure",
		"http 403", "http 429", "http 500", "http 502", "http 503", "http 504",
		"连接被重置", "连接被中断", "无法连接", "超时",
	} {
		if strings.Contains(text, signal) {
			return true
		}
	}
	return false
}

// hongguoPlaybackFailureClass 给诊断日志提供一个稳定的短分类，不包含任何
// 地址、参数或设备标识。
func hongguoPlaybackFailureClass(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, errHongguoPlaybackRegistration):
		return "registration"
	case errors.Is(err, errHongguoPlaybackEmpty):
		return "empty"
	case errors.Is(err, errHongguoPlaybackAuth):
		return "auth"
	case errors.Is(err, errHongguoPlaybackMalformed):
		return "malformed"
	case errors.Is(err, errHongguoPlaybackMissingEpisode):
		return "missing_episode"
	case errors.Is(err, errHongguoPlaybackPayloadEmpty):
		return "payload_empty"
	case errors.Is(err, errHongguoPlaybackData):
		return "data"
	}
	var backoff *requestBackoff
	if errors.As(err, &backoff) {
		return "backoff"
	}
	if hongguoPlaybackEgressBlocked(err) {
		return "egress"
	}
	return "transport"
}

// hongguoPlaybackUserMessage 生成最终展示给用户的失败原因。文案刻意不包含
// host、路径、查询参数或设备号。
func hongguoPlaybackUserMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errHongguoPlaybackRegistration) {
		return "红果匿名播放设备注册失败，请切换网络或稍后重试。"
	}
	var backoff *requestBackoff
	if errors.As(err, &backoff) {
		return fmt.Sprintf("红果播放服务拒绝了当前网络（HTTP %d），请切换网络或稍后重试。", backoff.status)
	}
	if errors.Is(err, errHongguoPlaybackMissingEpisode) {
		return "红果未返回该集片源，可能已下架或仅限试看；请换一集。"
	}
	if errors.Is(err, errHongguoPlaybackData) {
		return "红果返回的片源暂不可用，请切换线路或稍后重试。"
	}
	if errors.Is(err, errHongguoPlaybackEmpty) || errors.Is(err, errHongguoPlaybackAuth) {
		return "红果拒绝了本集的播放鉴权，已自动更换播放设备仍失败；请稍后重试或换一集。"
	}
	if errors.Is(err, errHongguoPlaybackMalformed) || errors.Is(err, errHongguoPlaybackPayloadEmpty) {
		return "红果返回的播放信息异常，已尝试重新请求；请稍后重试或换一集。"
	}
	if hongguoPlaybackEgressBlocked(err) {
		return "与红果服务器的连接被中断，已尝试兼容协议；请切换网络后重试。"
	}
	return publicError(err).Error()
}
