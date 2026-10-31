// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const hongguoPlaybackSuccessBody = `{"code":0,"data":{"123":{"video_model":{"video_list":[{"main_url":"https://tp3.douyinvod.com/video.mp4","codec_type":"h264","video_meta":{"vheight":1080,"codec_type":"h264"},"encrypt_info":{"encrypt":"false"}}]}}}}`

func hongguoPlaybackTestDownloader(t *testing.T, media func(*http.Request, int) (*http.Response, error)) (*Downloader, *int32, *int32) {
	t.Helper()
	var mediaCalls, registerCalls int32
	d := sourceFixtureDownloader(t, func(request *http.Request) (*http.Response, error) {
		switch request.URL.Hostname() {
		case "log5-applog-lf.fqnovel.com":
			call := atomic.AddInt32(&registerCalls, 1)
			id := strconv.Itoa(1000 + int(call))
			return sourceFixtureResponse(request, http.StatusOK, `{"device_id_str":"`+id+`","install_id_str":"`+id+`9"}`), nil
		case "api5-normal-sinfonlineb.fqnovel.com":
			call := int(atomic.AddInt32(&mediaCalls, 1))
			return media(request, call)
		default:
			return nil, fmt.Errorf("unexpected host %s", request.URL.Hostname())
		}
	})
	return d, &mediaCalls, &registerCalls
}

func TestHongguoPlaybackRegistrationFailureUsesCooldown(t *testing.T) {
	var calls int32
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	d := sourceFixtureDownloader(t, func(request *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return sourceFixtureResponse(request, http.StatusOK, `{"device_id":0}`), nil
	})
	client := d.hongguoClient()
	client.now = func() time.Time { return now }

	if _, err := d.hongguoPlaybackDevice(context.Background()); err == nil {
		t.Fatal("failed registration reported success")
	}
	if _, err := d.hongguoPlaybackDevice(context.Background()); err == nil {
		t.Fatal("cooldown failure reported success")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("cooldown did not suppress registration: %d", got)
	}

	now = now.Add(hongguoPlaybackCooldownBase + time.Second)
	if _, err := d.hongguoPlaybackDevice(context.Background()); err == nil {
		t.Fatal("retry after cooldown reported success")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("registration was not retried after cooldown: %d", got)
	}
}

func TestHongguoPlaybackRegistrationSingleflight(t *testing.T) {
	release := make(chan struct{})
	var calls int32
	d := sourceFixtureDownloader(t, func(request *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		<-release
		return sourceFixtureResponse(request, http.StatusOK, `{"device_id_str":"1001","install_id_str":"10019"}`), nil
	})

	const workers = 8
	var wait sync.WaitGroup
	results := make(chan hongguoPlaybackIdentity, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			identity, err := d.hongguoPlaybackDevice(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			results <- identity
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wait.Wait()
	close(results)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one shared registration, got %d", got)
	}
	var first hongguoPlaybackIdentity
	for identity := range results {
		if first == (hongguoPlaybackIdentity{}) {
			first = identity
		}
		if identity != first {
			t.Fatal("singleflight returned different identities")
		}
	}
}

func TestHongguoForgetOnlyClearsMatchingIdentity(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	d, _, registerCalls := hongguoPlaybackTestDownloader(t, func(request *http.Request, _ int) (*http.Response, error) {
		return sourceFixtureResponse(request, http.StatusOK, hongguoPlaybackSuccessBody), nil
	})
	client := d.hongguoClient()
	client.now = func() time.Time { return now }

	first, err := d.hongguoPlaybackDevice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !d.forgetHongguoPlaybackDevice(first) {
		t.Fatal("matching identity was not forgotten")
	}
	now = now.Add(hongguoPlaybackRotationInterval + time.Second)
	second, err := d.hongguoPlaybackDevice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("forgotten identity was reused")
	}
	if d.forgetHongguoPlaybackDevice(first) {
		t.Fatal("stale identity cleared the newly registered identity")
	}
	again, err := d.hongguoPlaybackDevice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if again != second {
		t.Fatal("new identity was not kept")
	}
	if got := atomic.LoadInt32(registerCalls); got != 2 {
		t.Fatalf("unexpected registration count %d", got)
	}
}

func TestHongguoRotationIntervalSuppressesStorms(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	d, _, _ := hongguoPlaybackTestDownloader(t, func(request *http.Request, _ int) (*http.Response, error) {
		return sourceFixtureResponse(request, http.StatusOK, hongguoPlaybackSuccessBody), nil
	})
	client := d.hongguoClient()
	client.now = func() time.Time { return now }

	first, err := d.hongguoPlaybackDevice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !d.forgetHongguoPlaybackDevice(first) {
		t.Fatal("first rotation was suppressed")
	}
	second, err := d.hongguoPlaybackDevice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.forgetHongguoPlaybackDevice(second) {
		t.Fatal("second rotation inside the interval was allowed")
	}
}

func TestHongguoPlaybackEmptyResponseRetriesSignedThenRotates(t *testing.T) {
	d, mediaCalls, registerCalls := hongguoPlaybackTestDownloader(t, func(request *http.Request, call int) (*http.Response, error) {
		if call <= 2 {
			return sourceFixtureResponse(request, http.StatusOK, ""), nil
		}
		return sourceFixtureResponse(request, http.StatusOK, hongguoPlaybackSuccessBody), nil
	})
	media, err := d.resolveHongguoRegisteredMedia(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(media.URL, "https://tp3.douyinvod.com/") {
		t.Fatalf("unexpected media url %q", media.URL)
	}
	if got := atomic.LoadInt32(mediaCalls); got != 3 {
		t.Fatalf("expected unsigned, signed and rotated attempts, got %d", got)
	}
	if got := atomic.LoadInt32(registerCalls); got != 2 {
		t.Fatalf("expected one fresh registration, got %d", got)
	}
}

func TestHongguoPlayback403DoesNotResignOrRotate(t *testing.T) {
	d, mediaCalls, registerCalls := hongguoPlaybackTestDownloader(t, func(request *http.Request, _ int) (*http.Response, error) {
		return sourceFixtureResponse(request, http.StatusForbidden, "forbidden"), nil
	})
	_, err := d.resolveHongguoRegisteredMedia(context.Background(), "123")
	if err == nil {
		t.Fatal("403 became a success")
	}
	var backoff *requestBackoff
	if !errors.As(err, &backoff) {
		t.Fatalf("403 was not classified as backoff: %v", err)
	}
	if got := atomic.LoadInt32(mediaCalls); got != 1 {
		t.Fatalf("403 caused extra media requests: %d", got)
	}
	if got := atomic.LoadInt32(registerCalls); got != 1 {
		t.Fatalf("403 caused re-registration: %d", got)
	}
}

func TestHongguoPlayback401RetriesSignedWithoutRotation(t *testing.T) {
	d, mediaCalls, registerCalls := hongguoPlaybackTestDownloader(t, func(request *http.Request, call int) (*http.Response, error) {
		if call == 1 {
			return sourceFixtureResponse(request, http.StatusUnauthorized, ""), nil
		}
		return sourceFixtureResponse(request, http.StatusOK, hongguoPlaybackSuccessBody), nil
	})
	media, err := d.resolveHongguoRegisteredMedia(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if media.URL == "" {
		t.Fatal("missing media url")
	}
	if got := atomic.LoadInt32(mediaCalls); got != 2 {
		t.Fatalf("401 did not trigger a signed retry: %d", got)
	}
	if got := atomic.LoadInt32(registerCalls); got != 1 {
		t.Fatalf("401 triggered an unnecessary rotation: %d", got)
	}
}

func TestHongguoPlaybackMissingEpisodeDoesNotRotate(t *testing.T) {
	d, mediaCalls, registerCalls := hongguoPlaybackTestDownloader(t, func(request *http.Request, _ int) (*http.Response, error) {
		return sourceFixtureResponse(request, http.StatusOK, `{"code":0,"data":{}}`), nil
	})
	_, err := d.resolveHongguoRegisteredMedia(context.Background(), "123")
	if !errors.Is(err, errHongguoPlaybackMissingEpisode) {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(mediaCalls); got != 1 {
		t.Fatalf("missing episode caused extra requests: %d", got)
	}
	if got := atomic.LoadInt32(registerCalls); got != 1 {
		t.Fatalf("missing episode caused re-registration: %d", got)
	}
}

func TestHongguoPlaybackUserMessagesHideInternals(t *testing.T) {
	registration := &hongguoPlaybackRegistrationError{cause: errors.New("https://example.test/path?token=secret")}
	message := hongguoPlaybackUserMessage(registration)
	if strings.Contains(message, "example.test") || strings.Contains(message, "token=") {
		t.Fatalf("registration message leaked internals: %q", message)
	}
	if !strings.Contains(message, "稍后重试") {
		t.Fatalf("unexpected registration message: %q", message)
	}

	auth := hongguoPlaybackClassify(errHongguoPlaybackAuth, "https://example.test/path?token=secret")
	message = hongguoPlaybackUserMessage(auth)
	if strings.Contains(message, "example.test") || strings.Contains(message, "token=") {
		t.Fatalf("auth message leaked internals: %q", message)
	}

	backoff := &requestBackoff{host: "api5.example.test", until: time.Now().Add(time.Minute), status: http.StatusForbidden}
	message = hongguoPlaybackUserMessage(backoff)
	if !strings.Contains(message, "HTTP 403") {
		t.Fatalf("backoff message lost the status: %q", message)
	}
}

func TestHongguoPlaybackAuthRetryableClassification(t *testing.T) {
	for _, err := range []error{
		errHongguoPlaybackEmpty,
		hongguoPlaybackClassify(errHongguoPlaybackAuth, "401"),
		errHongguoPlaybackMalformed,
		hongguoPlaybackClassify(errHongguoPlaybackPayloadEmpty, "no model"),
		&hongguoPlaybackCodeError{code: "1001"},
	} {
		if !hongguoPlaybackAuthRetryable(err) {
			t.Fatalf("expected auth retry for %v", err)
		}
	}
	for _, err := range []error{
		nil,
		errHongguoPlaybackMissingEpisode,
		hongguoPlaybackClassify(errHongguoPlaybackData, "no key"),
		&requestBackoff{host: "api5.example.test", until: time.Now().Add(time.Minute), status: http.StatusForbidden},
		context.Canceled,
		errors.New("connection reset by peer"),
	} {
		if hongguoPlaybackAuthRetryable(err) {
			t.Fatalf("unexpected auth retry for %v", err)
		}
	}
}

func TestHongguoALPNHostAndRetryable(t *testing.T) {
	if !hongguoALPNHost("api5-normal-sinfonlineb.fqnovel.com") || !hongguoALPNHost("log.snssdk.com") {
		t.Fatal("hongguo hosts were not recognized")
	}
	if hongguoALPNHost("djapi.999888456.xyz") {
		t.Fatal("removed fallback host is still recognized")
	}
	if !hongguoALPNRetryable(errors.New("tls: no application protocol")) {
		t.Fatal("ALPN failure was not retryable")
	}
	if !hongguoALPNRetryable(errors.New("connection reset by peer")) {
		t.Fatal("connection reset was not retryable")
	}
	for _, err := range []error{
		context.Canceled,
		&net.DNSError{Err: "no such host", Name: "example.test"},
		&tls.CertificateVerificationError{Err: errors.New("x509: unknown authority")},
		errors.New("i/o timeout"),
	} {
		if hongguoALPNRetryable(err) {
			t.Fatalf("unexpected ALPN retry for %v", err)
		}
	}
}

func TestHongguoMediaEdgeHosts(t *testing.T) {
	family, hosts, ok := hongguoMediaEdgeHosts("https://tp3.douyinvod.com/video/1.mp4?sign=a")
	if !ok || family != "tp*.douyinvod.com" {
		t.Fatalf("unexpected edge family %q ok=%v", family, ok)
	}
	if len(hosts) != 8 || hosts[0] != "tp3.douyinvod.com" || hosts[1] != "tp1.douyinvod.com" || hosts[2] != "tp4.douyinvod.com" {
		t.Fatalf("unexpected edge order %v", hosts)
	}
	_, hosts, ok = hongguoMediaEdgeHosts("https://tp3-dy.fqnovelvod.com/video/1.mp4")
	if !ok || hosts[0] != "tp3-dy.fqnovelvod.com" || hosts[1] != "tp1-dy.fqnovelvod.com" {
		t.Fatalf("unexpected -dy edges %v ok=%v", hosts, ok)
	}
	for _, raw := range []string{
		"https://v26.douyinvod.com/video/1.mp4",
		"http://tp3.douyinvod.com/video/1.mp4",
		"https://tp9.douyinvod.com/video/1.mp4",
		"https://tp3.example.com/video/1.mp4",
	} {
		if _, _, ok := hongguoMediaEdgeHosts(raw); ok {
			t.Fatalf("unexpected edge match for %s", raw)
		}
	}
}

func TestHongguoEdgeCandidatesPreferSuccessAndForceOriginal(t *testing.T) {
	now := time.Now()
	stream := &nativeStreamServer{
		edgePreferred: map[string]hongguoEdgeRoute{"tp*.douyinvod.com": {host: "tp4.douyinvod.com", expires: now.Add(time.Minute)}},
		edgeBackoff:   map[string]time.Time{"tp1.douyinvod.com": now.Add(time.Minute)},
	}
	hosts := stream.hongguoEdgeCandidates("tp*.douyinvod.com", "tp3.douyinvod.com", []string{"tp3.douyinvod.com", "tp1.douyinvod.com", "tp4.douyinvod.com"})
	if len(hosts) < 2 || hosts[0] != "tp4.douyinvod.com" || hosts[1] != "tp3.douyinvod.com" {
		t.Fatalf("unexpected candidate order %v", hosts)
	}
	for _, host := range hosts {
		if host == "tp1.douyinvod.com" {
			t.Fatal("backed-off edge was offered")
		}
	}
	stream.edgeBackoff["tp3.douyinvod.com"] = now.Add(time.Minute)
	hosts = stream.hongguoEdgeCandidates("tp*.douyinvod.com", "tp3.douyinvod.com", []string{"tp3.douyinvod.com"})
	found := false
	for _, host := range hosts {
		if host == "tp3.douyinvod.com" {
			found = true
		}
	}
	if !found {
		t.Fatal("original host was not forced after backoff")
	}
}

func TestClearHongguoBackoffsKeepsOtherSources(t *testing.T) {
	limiter := newRequestLimiter(2, 0)
	limiter.backoffs["api5-normal-sinfonlineb.fqnovel.com"] = &requestBackoff{host: "api5-normal-sinfonlineb.fqnovel.com", until: time.Now().Add(time.Minute), status: 403}
	limiter.backoffs["example.test"] = &requestBackoff{host: "example.test", until: time.Now().Add(time.Minute), status: 403}
	limiter.clearHongguoBackoffs()
	if _, ok := limiter.backoffs["api5-normal-sinfonlineb.fqnovel.com"]; ok {
		t.Fatal("Hongguo backoff was not cleared")
	}
	if _, ok := limiter.backoffs["example.test"]; !ok {
		t.Fatal("unrelated backoff was cleared")
	}
}
