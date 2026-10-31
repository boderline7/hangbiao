package sub

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ACLMirrors 是 GitHub raw 的加速前缀，空串表示直连。探测时按可用性自动选择。
var ACLMirrors = []string{
	"",
	"https://ghproxy.net/",
	"https://gh-proxy.com/",
}

const (
	githubRawHost = "raw.githubusercontent.com"
	probeURL      = "https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/BanAD.list"
	probeTimeout  = 8 * time.Second
	fetchTimeout  = 20 * time.Second
	maxBody       = 8 << 20
	userAgent     = "hangbiao"
)

var (
	mirrorMu   sync.RWMutex
	mirror     string
	mirrorAt   time.Time
	mirrorBusy bool
)

var httpClient = &http.Client{Timeout: fetchTimeout}

// IsGitHubRaw 判断是否为 GitHub raw 链接（这类链接在部分网络里不可达）。
func IsGitHubRaw(u string) bool {
	return strings.Contains(u, "://"+githubRawHost+"/")
}

// Mirror 返回最近一次探测到的加速前缀（空串为直连）。不会发起网络请求。
func Mirror() string {
	mirrorMu.RLock()
	defer mirrorMu.RUnlock()
	return mirror
}

// MirrorAt 返回上次探测时间。
func MirrorAt() time.Time {
	mirrorMu.RLock()
	defer mirrorMu.RUnlock()
	return mirrorAt
}

// RewriteURL 给 GitHub raw 链接套上当前可用的加速前缀，其他链接原样返回。
func RewriteURL(u string) string {
	if !IsGitHubRaw(u) {
		return u
	}
	if m := Mirror(); m != "" {
		return m + u
	}
	return u
}

func setMirror(prefix string) {
	mirrorMu.Lock()
	mirror, mirrorAt = prefix, time.Now()
	mirrorMu.Unlock()
}

// Detect 并发探测候选源，取最先成功的那个并缓存；全部失败时保留原值。
func Detect(ctx context.Context) string {
	mirrorMu.Lock()
	if mirrorBusy {
		m := mirror
		mirrorMu.Unlock()
		return m
	}
	mirrorBusy = true
	mirrorMu.Unlock()
	defer func() {
		mirrorMu.Lock()
		mirrorBusy = false
		mirrorMu.Unlock()
	}()

	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	type result struct{ prefix string }
	ch := make(chan result, len(ACLMirrors))
	for _, prefix := range ACLMirrors {
		go func(prefix string) {
			if _, err := get(probeCtx, prefix+probeURL); err == nil {
				ch <- result{prefix}
			}
		}(prefix)
	}
	for range ACLMirrors {
		select {
		case r := <-ch:
			setMirror(r.prefix)
			return r.prefix
		case <-probeCtx.Done():
			return Mirror()
		}
	}
	return Mirror()
}

// Fetch 拉取规则内容：优先用缓存源，失败后依次尝试其他源并记住可用的那个。
func Fetch(ctx context.Context, raw string) ([]byte, string, error) {
	if !IsGitHubRaw(raw) {
		body, err := get(ctx, raw)
		return body, "", err
	}
	cached := Mirror()
	order := make([]string, 0, len(ACLMirrors))
	if cached != "" {
		order = append(order, cached)
	}
	for _, p := range ACLMirrors {
		if p != cached {
			order = append(order, p)
		}
	}
	var lastErr error
	for _, prefix := range order {
		body, err := get(ctx, prefix+raw)
		if err == nil {
			if prefix != cached {
				setMirror(prefix)
			}
			return body, prefix, nil
		}
		lastErr = err
	}
	return nil, "", lastErr
}

func get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("响应为空")
	}
	return body, nil
}
