// Package sub 解析机场订阅：Clash YAML、Base64 分享链接列表、明文分享链接。
package sub

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"towerdocker/internal/model"
)

var ErrUnknownFormat = errors.New("无法识别的订阅格式")

type clashDoc struct {
	Proxies []model.Proxy `yaml:"proxies"`
}

// Parse 返回节点列表与识别出的格式名。
func Parse(body []byte) ([]model.Proxy, string, error) {
	text := strings.TrimSpace(strings.TrimPrefix(string(body), "\ufeff"))
	if text == "" {
		return nil, "", ErrUnknownFormat
	}

	if doc, ok := tryClash(text); ok {
		return doc, "clash", nil
	}

	if decoded, ok := tryBase64(text); ok {
		if nodes := parseLines(decoded); len(nodes) > 0 {
			return nodes, "base64", nil
		}
	}

	if nodes := parseLines(text); len(nodes) > 0 {
		return nodes, "uri", nil
	}

	return nil, "", ErrUnknownFormat
}

func tryClash(text string) ([]model.Proxy, bool) {
	if !strings.Contains(text, "proxies:") {
		return nil, false
	}
	var doc clashDoc
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, false
	}
	var out []model.Proxy
	for _, p := range doc.Proxies {
		if p.Name() != "" && p.String("type") != "" {
			out = append(out, p)
		}
	}
	return out, len(out) > 0
}

func tryBase64(text string) (string, bool) {
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, text)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(clean); err == nil {
			s := string(b)
			if strings.Contains(s, "://") {
				return s, true
			}
		}
	}
	return "", false
}

func parseLines(text string) []model.Proxy {
	var out []model.Proxy
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if !strings.Contains(line, "://") {
			continue
		}
		if p := parseURI(line); p != nil {
			out = append(out, p)
		}
	}
	return out
}

func parseURI(raw string) model.Proxy {
	switch {
	case strings.HasPrefix(raw, "vmess://"):
		return parseVmess(raw)
	case strings.HasPrefix(raw, "ss://"):
		return parseSS(raw)
	case strings.HasPrefix(raw, "trojan://"):
		return parseTrojan(raw)
	case strings.HasPrefix(raw, "vless://"):
		return parseVless(raw)
	case strings.HasPrefix(raw, "hysteria2://"), strings.HasPrefix(raw, "hy2://"):
		return parseHy2(raw)
	case strings.HasPrefix(raw, "hysteria://"):
		return parseHysteria1(raw)
	case strings.HasPrefix(raw, "tuic://"):
		return parseTuic(raw)
	case strings.HasPrefix(raw, "anytls://"):
		return parseAnyTLS(raw)
	}
	return nil
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func decodeName(frag string) string {
	if frag == "" {
		return ""
	}
	if s, err := url.QueryUnescape(frag); err == nil {
		return s
	}
	return frag
}

func splitHostPort(hp string) (string, int) {
	host, portStr, ok := strings.Cut(hp, ":")
	if !ok {
		return hp, 0
	}
	p, _ := strconv.Atoi(portStr)
	return host, p
}

func b64decode(s string) (string, bool) {
	s = strings.TrimRight(s, "=")
	for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding, base64.StdEncoding, base64.URLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return string(b), true
		}
	}
	return "", false
}

func parseVmess(raw string) model.Proxy {
	payload := strings.TrimPrefix(raw, "vmess://")
	if i := strings.Index(payload, "#"); i >= 0 {
		payload = payload[:i]
	}
	js, ok := b64decode(payload)
	if !ok {
		return nil
	}
	var v struct {
		PS   string `json:"ps"`
		Add  string `json:"add"`
		Port any    `json:"port"`
		ID   string `json:"id"`
		Aid  any    `json:"aid"`
		Net  string `json:"net"`
		Host string `json:"host"`
		Path string `json:"path"`
		TLS  string `json:"tls"`
		SNI  string `json:"sni"`
		Scy  string `json:"scy"`
	}
	if err := json.Unmarshal([]byte(js), &v); err != nil || v.Add == "" || v.ID == "" {
		return nil
	}
	p := model.Proxy{
		"name":    orDefault(v.PS, v.Add),
		"type":    "vmess",
		"server":  v.Add,
		"port":    anyInt(v.Port),
		"uuid":    v.ID,
		"alterId": anyInt(v.Aid),
		"cipher":  orDefault(v.Scy, "auto"),
		"udp":     true,
	}
	if v.TLS == "tls" {
		p["tls"] = true
		if v.SNI != "" {
			p["servername"] = v.SNI
		} else if v.Host != "" {
			p["servername"] = v.Host
		}
	}
	switch v.Net {
	case "ws":
		p["network"] = "ws"
		ws := map[string]any{"path": orDefault(v.Path, "/")}
		if v.Host != "" {
			ws["headers"] = map[string]any{"Host": v.Host}
		}
		p["ws-opts"] = ws
	case "grpc":
		p["network"] = "grpc"
		p["grpc-opts"] = map[string]any{"grpc-service-name": v.Path}
	case "h2":
		p["network"] = "h2"
		p["h2-opts"] = map[string]any{"path": orDefault(v.Path, "/"), "host": []string{v.Host}}
	}
	return p
}

func parseSS(raw string) model.Proxy {
	body := strings.TrimPrefix(raw, "ss://")
	name := ""
	if i := strings.Index(body, "#"); i >= 0 {
		name = decodeName(body[i+1:])
		body = body[:i]
	}
	if i := strings.Index(body, "?"); i >= 0 {
		body = body[:i]
	}
	var method, password, hostport string
	if at := strings.LastIndex(body, "@"); at >= 0 {
		cred := body[:at]
		hostport = body[at+1:]
		if dec, ok := b64decode(cred); ok && strings.Contains(dec, ":") {
			cred = dec
		}
		m, p, ok := strings.Cut(cred, ":")
		if !ok {
			return nil
		}
		method, password = m, p
	} else {
		dec, ok := b64decode(body)
		if !ok {
			return nil
		}
		cred, hp, ok := strings.Cut(dec, "@")
		if !ok {
			return nil
		}
		m, p, ok := strings.Cut(cred, ":")
		if !ok {
			return nil
		}
		method, password, hostport = m, p, hp
	}
	host, port := splitHostPort(hostport)
	if host == "" || port == 0 {
		return nil
	}
	return model.Proxy{
		"name":     orDefault(name, host),
		"type":     "ss",
		"server":   host,
		"port":     port,
		"cipher":   method,
		"password": password,
		"udp":      true,
	}
}

func parseTrojan(raw string) model.Proxy {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}
	p := model.Proxy{
		"name":     orDefault(decodeName(u.Fragment), u.Hostname()),
		"type":     "trojan",
		"server":   u.Hostname(),
		"port":     port,
		"password": u.User.Username(),
		"udp":      true,
	}
	q := u.Query()
	if sni := q.Get("sni"); sni != "" {
		p["sni"] = sni
	}
	if q.Get("allowInsecure") == "1" {
		p["skip-cert-verify"] = true
	}
	applyTransport(p, q)
	return p
}

func parseVless(raw string) model.Proxy {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}
	p := model.Proxy{
		"name":   orDefault(decodeName(u.Fragment), u.Hostname()),
		"type":   "vless",
		"server": u.Hostname(),
		"port":   port,
		"uuid":   u.User.Username(),
		"udp":    true,
	}
	q := u.Query()
	if flow := q.Get("flow"); flow != "" {
		p["flow"] = flow
	}
	if q.Get("security") == "tls" || q.Get("security") == "reality" {
		p["tls"] = true
		if sni := q.Get("sni"); sni != "" {
			p["servername"] = sni
		}
		if q.Get("security") == "reality" {
			p["reality-opts"] = map[string]any{
				"public-key": q.Get("pbk"),
				"short-id":   q.Get("sid"),
			}
			p["client-fingerprint"] = orDefault(q.Get("fp"), "chrome")
		}
	}
	applyTransport(p, q)
	return p
}

func parseHy2(raw string) model.Proxy {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}
	q := u.Query()
	p := model.Proxy{
		"name":     orDefault(decodeName(u.Fragment), u.Hostname()),
		"type":     "hysteria2",
		"server":   u.Hostname(),
		"port":     port,
		"password": u.User.Username(),
	}
	if sni := orDefault(q.Get("sni"), q.Get("peer")); sni != "" {
		p["sni"] = sni
	}
	if truthy(q.Get("insecure")) || truthy(q.Get("allowInsecure")) {
		p["skip-cert-verify"] = true
	}
	if obfs := q.Get("obfs"); obfs != "" {
		p["obfs"] = obfs
		pass := q.Get("obfs-password")
		if pass == "" {
			pass = q.Get("obfs_password")
		}
		if pass == "" {
			pass = q.Get("obfsParam")
		}
		if pass != "" {
			p["obfs-password"] = pass
		}
	}
	if pin := orDefault(q.Get("pinSHA256"), q.Get("pinsha256")); pin != "" {
		p["fingerprint"] = pin
	}
	if alpn := q.Get("alpn"); alpn != "" {
		p["alpn"] = splitList(alpn)
	}
	if up := q.Get("up"); up != "" {
		p["up"] = up
	}
	if down := q.Get("down"); down != "" {
		p["down"] = down
	}
	return p
}

// parseHysteria1 解析 hysteria v1：hysteria://host:port?auth=***&peer=sni&upmbps=&downmbps=#名字
func parseHysteria1(raw string) model.Proxy {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}
	q := u.Query()
	p := model.Proxy{
		"name":   orDefault(decodeName(u.Fragment), u.Hostname()),
		"type":   "hysteria",
		"server": u.Hostname(),
		"port":   port,
	}
	if auth := orDefault(q.Get("auth"), q.Get("auth_str")); auth != "" {
		p["auth-str"] = auth
	}
	if peer := orDefault(q.Get("peer"), q.Get("sni")); peer != "" {
		p["sni"] = peer
	}
	if truthy(q.Get("insecure")) || truthy(q.Get("allowInsecure")) {
		p["skip-cert-verify"] = true
	}
	if up := orDefault(q.Get("upmbps"), q.Get("up")); up != "" {
		p["up"] = up
	}
	if down := orDefault(q.Get("downmbps"), q.Get("down")); down != "" {
		p["down"] = down
	}
	if proto := q.Get("protocol"); proto != "" {
		p["protocol"] = proto
	}
	if obfs := q.Get("obfs"); obfs != "" {
		p["obfs"] = obfs
	}
	return p
}

// parseTuic 解析 TUIC v5：tuic://uuid:password@host:port?congestion_control=bbr&alpn=h3&sni=#名字
func parseTuic(raw string) model.Proxy {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User == nil {
		return nil
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}
	password, _ := u.User.Password()
	q := u.Query()
	p := model.Proxy{
		"name":     orDefault(decodeName(u.Fragment), u.Hostname()),
		"type":     "tuic",
		"server":   u.Hostname(),
		"port":     port,
		"uuid":     u.User.Username(),
		"password": password,
		"alpn":     splitList(orDefault(q.Get("alpn"), "h3")),
	}
	if cc := orDefault(q.Get("congestion_control"), q.Get("congestion-controller")); cc != "" {
		p["congestion-controller"] = cc
	}
	if mode := orDefault(q.Get("udp_relay_mode"), q.Get("udp-relay-mode")); mode != "" {
		p["udp-relay-mode"] = mode
	}
	if truthy(q.Get("reduce_rtt")) || truthy(q.Get("reduce-rtt")) {
		p["reduce-rtt"] = true
	}
	if sni := q.Get("sni"); sni != "" {
		p["sni"] = sni
	}
	if truthy(q.Get("allow_insecure")) || truthy(q.Get("insecure")) {
		p["skip-cert-verify"] = true
	}
	return p
}

// parseAnyTLS 解析 anytls://password@host:port?sni=***&insecure=1#名字
func parseAnyTLS(raw string) model.Proxy {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}
	q := u.Query()
	p := model.Proxy{
		"name":   orDefault(decodeName(u.Fragment), u.Hostname()),
		"type":   "anytls",
		"server": u.Hostname(),
		"port":   port,
	}
	if u.User != nil {
		p["password"] = u.User.Username()
	}
	if sni := q.Get("sni"); sni != "" {
		p["sni"] = sni
	}
	if truthy(q.Get("insecure")) || truthy(q.Get("allowInsecure")) {
		p["skip-cert-verify"] = true
	}
	if fp := q.Get("fp"); fp != "" {
		p["client-fingerprint"] = fp
	}
	return p
}

func applyTransport(p model.Proxy, q url.Values) {
	switch q.Get("type") {
	case "ws":
		p["network"] = "ws"
		ws := map[string]any{"path": orDefault(q.Get("path"), "/")}
		if h := q.Get("host"); h != "" {
			ws["headers"] = map[string]any{"Host": h}
		}
		p["ws-opts"] = ws
	case "grpc":
		p["network"] = "grpc"
		p["grpc-opts"] = map[string]any{"grpc-service-name": q.Get("serviceName")}
	}
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func anyInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	case int:
		return n
	}
	return 0
}
