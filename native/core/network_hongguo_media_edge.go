// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	hongguoMediaEdgePreferredTTL = 10 * time.Minute
	hongguoMediaEdgeBackoff      = 30 * time.Second
	hongguoMediaEdgeMaxAttempts  = 4
)

var hongguoMediaEdgeOrder = []int{1, 4, 7, 8, 2, 3, 5, 6}

// hongguoMediaEdgeHosts 只识别 tp1..tp8 形式的红果媒体 host，例如
// tp3.douyinvod.com 或 tp3-dy.fqnovelvod.com。原 host 永远排在候选
// 第一位；如果真实地址不是这种形式，函数返回 ok=false，代理行为与
// 以前完全一致。
func hongguoMediaEdgeHosts(rawURL string) (string, []string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return "", nil, false
	}
	host := strings.ToLower(parsed.Hostname())
	label, parent := host, ""
	if index := strings.IndexByte(host, '.'); index > 0 {
		label, parent = host[:index], host[index+1:]
	}
	if len(label) < 3 || !strings.HasPrefix(label, "tp") {
		return "", nil, false
	}
	edge, err := strconv.Atoi(label[2:3])
	if err != nil || edge < 1 || edge > 8 {
		return "", nil, false
	}
	rest := label[3:]
	if rest != "" && !strings.HasPrefix(rest, "-") {
		return "", nil, false
	}
	if !hongguoMediaEdgeDomain(parent) {
		return "", nil, false
	}
	family := "tp*" + rest + "." + parent
	candidates := []string{host}
	for _, candidateEdge := range hongguoMediaEdgeOrder {
		candidate := fmt.Sprintf("tp%d%s.%s", candidateEdge, rest, parent)
		if candidate == host {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return family, candidates, true
}

func hongguoMediaEdgeDomain(parent string) bool {
	for _, domain := range []string{"qznovelvod.com", "fqnovelvod.com", "douyinvod.com"} {
		if parent == domain {
			return true
		}
	}
	return false
}

func hongguoMediaEdgeRetryStatus(status int) bool {
	switch status {
	case 403, 429, 500, 502, 503, 504:
		return true
	}
	return false
}
