package exporter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"towerdocker/internal/model"
)

// PassWall 是 OpenWrt 上的分流工具，它读不了 Clash YAML，只认分享链接订阅：
// 一份 Base64 编码的链接列表，一行一个。订阅里只有节点，没有策略组和规则——
// 分流由 PassWall 自己的规则在路由器上完成，这部分无法从本项目的分组带过去。
//
// 输出样例（解码后）：
//
//	ss://YWVzLTEyOC1nY206cHc@1.2.3.4:8388#%F0%9F%87%AD%F0%9F%87%B0%20%E9%A6%99%E6%B8%AF
//	vmess://eyJ2IjoiMiIsInBzIjoi...
func PassWall(st *model.State) Result {
	return shareSubscription(st, "passwall")
}

func shareSubscription(st *model.State, slug string) Result {
	var warnings []string
	lines := make([]string, 0, len(st.Nodes()))
	for _, p := range st.Nodes() {
		link, ok := passwallLink(p)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("分享链接不支持 %s（%s），已跳过", p.Name(), p.String("type")))
			continue
		}
		lines = append(lines, link)
	}
	text := strings.Join(lines, "\n")
	if text != "" {
		text += "\n"
	}
	body := base64.StdEncoding.EncodeToString([]byte(text))

	return Result{
		Body:     []byte(body),
		Filename: safeName(st.Settings.Title) + "-" + slug + ".txt",
		Warnings: dedupe(warnings),
		Kept:     len(lines),
	}
}

func passwallLink(p model.Proxy) (string, bool) {
	host := p.String("server")
	port := p.Int("port")
	if host == "" || port == 0 {
		return "", false
	}
	name := url.PathEscape(p.Name())
	addr := host + ":" + strconv.Itoa(port)
	typ := strings.ToLower(p.String("type"))

	switch typ {
	case "ss":
		if p.String("cipher") == "" || p.String("password") == "" {
			return "", false
		}
		cred := base64.RawURLEncoding.EncodeToString([]byte(p.String("cipher") + ":" + p.String("password")))
		link := "ss://" + cred + "@" + addr
		if q := ssPluginQuery(p); q != "" {
			link += "?" + q
		}
		return link + "#" + name, true

	case "vmess":
		if p.String("uuid") == "" {
			return "", false
		}
		v := map[string]string{
			"v":    "2",
			"ps":   p.Name(),
			"add":  host,
			"port": strconv.Itoa(port),
			"id":   p.String("uuid"),
			"aid":  strconv.Itoa(p.Int("alterId")),
			"scy":  orDefaultStr(p.String("cipher"), "auto"),
			"net":  orDefaultStr(p.String("network"), "tcp"),
			"type": "none",
			"host": "",
			"path": "",
			"tls":  "",
			"sni":  sniOf(p),
		}
		fillTransport(v, p)
		if p.Bool("tls") {
			v["tls"] = "tls"
		}
		js, err := json.Marshal(v)
		if err != nil {
			return "", false
		}
		return "vmess://" + base64.StdEncoding.EncodeToString(js), true

	case "vless":
		if p.String("uuid") == "" {
			return "", false
		}
		q := url.Values{}
		q.Set("encryption", "none")
		setTLSQuery(q, p)
		setTransportQuery(q, p)
		if flow := p.String("flow"); flow != "" {
			q.Set("flow", flow)
		}
		return "vless://" + url.PathEscape(p.String("uuid")) + "@" + addr + "?" + q.Encode() + "#" + name, true

	case "trojan":
		if p.String("password") == "" {
			return "", false
		}
		q := url.Values{}
		if sni := sniOf(p); sni != "" {
			q.Set("sni", sni)
		}
		if p.Bool("skip-cert-verify") {
			q.Set("allowInsecure", "1")
		}
		setTransportQuery(q, p)
		link := "trojan://" + url.PathEscape(p.String("password")) + "@" + addr
		if len(q) > 0 {
			link += "?" + q.Encode()
		}
		return link + "#" + name, true

	case "hysteria2":
		if p.String("password") == "" {
			return "", false
		}
		q := url.Values{}
		if sni := sniOf(p); sni != "" {
			q.Set("sni", sni)
		}
		if p.Bool("skip-cert-verify") {
			q.Set("insecure", "1")
		}
		if obfs := p.String("obfs"); obfs != "" {
			q.Set("obfs", obfs)
			if pass := p.String("obfs-password"); pass != "" {
				q.Set("obfs-password", pass)
			}
		}
		return "hysteria2://" + url.PathEscape(p.String("password")) + "@" + addr + "?" + q.Encode() + "#" + name, true

	case "hysteria":
		q := url.Values{}
		if auth := p.String("auth-str"); auth != "" {
			q.Set("auth", auth)
		}
		if sni := sniOf(p); sni != "" {
			q.Set("peer", sni)
		}
		if p.Bool("skip-cert-verify") {
			q.Set("insecure", "1")
		}
		if up := p.String("up"); up != "" {
			q.Set("upmbps", up)
		}
		if down := p.String("down"); down != "" {
			q.Set("downmbps", down)
		}
		if proto := p.String("protocol"); proto != "" {
			q.Set("protocol", proto)
		}
		if obfs := p.String("obfs"); obfs != "" {
			q.Set("obfs", obfs)
		}
		return "hysteria://" + addr + "?" + q.Encode() + "#" + name, true

	case "tuic":
		q := url.Values{}
		if cc := p.String("congestion-controller"); cc != "" {
			q.Set("congestion_control", cc)
		}
		if mode := p.String("udp-relay-mode"); mode != "" {
			q.Set("udp_relay_mode", mode)
		}
		q.Set("alpn", strings.Join(alpnOf(p), ","))
		if sni := sniOf(p); sni != "" {
			q.Set("sni", sni)
		}
		if p.Bool("skip-cert-verify") {
			q.Set("allow_insecure", "1")
		}
		user := url.PathEscape(p.String("uuid")) + ":" + url.PathEscape(p.String("password"))
		return "tuic://" + user + "@" + addr + "?" + q.Encode() + "#" + name, true

	case "ssr":
		// ssr://base64(host:port:protocol:method:obfs:base64(password)/?params)
		if p.String("cipher") == "" || p.String("password") == "" {
			return "", false
		}
		pwd := base64.RawURLEncoding.EncodeToString([]byte(p.String("password")))
		body := fmt.Sprintf("%s:%d:%s:%s:%s:%s", host, port,
			orDefaultStr(p.String("protocol"), "origin"),
			p.String("cipher"),
			orDefaultStr(p.String("obfs"), "plain"),
			pwd)
		q := url.Values{}
		q.Set("remarks", base64.RawURLEncoding.EncodeToString([]byte(p.Name())))
		if v := p.String("obfs-param"); v != "" {
			q.Set("obfsparam", base64.RawURLEncoding.EncodeToString([]byte(v)))
		}
		if v := p.String("protocol-param"); v != "" {
			q.Set("protoparam", base64.RawURLEncoding.EncodeToString([]byte(v)))
		}
		return "ssr://" + base64.RawURLEncoding.EncodeToString([]byte(body+"/?"+q.Encode())), true

	case "anytls":
		q := url.Values{}
		if sni := sniOf(p); sni != "" {
			q.Set("sni", sni)
		}
		if p.Bool("skip-cert-verify") {
			q.Set("insecure", "1")
		}
		if fp := p.String("client-fingerprint"); fp != "" {
			q.Set("fp", fp)
		}
		link := "anytls://" + url.PathEscape(p.String("password")) + "@" + addr
		if len(q) > 0 {
			link += "?" + q.Encode()
		}
		return link + "#" + name, true
	}
	return "", false
}

// ssPluginQuery 生成 SIP003 插件串，PassWall 的 ss 节点支持 obfs 与 v2ray-plugin。
func ssPluginQuery(p model.Proxy) string {
	opts, ok := p["plugin-opts"].(map[string]any)
	if !ok {
		return ""
	}
	plugin := strings.ToLower(p.String("plugin"))
	mode, _ := opts["mode"].(string)
	host, _ := opts["host"].(string)
	switch {
	case plugin == "obfs" || (plugin == "" && mode != ""):
		parts := []string{"obfs-local", "obfs=" + orDefaultStr(mode, "http")}
		if host != "" {
			parts = append(parts, "obfs-host="+host)
		}
		return "plugin=" + url.QueryEscape(strings.Join(parts, ";"))
	case strings.Contains(plugin, "v2ray"):
		parts := []string{"v2ray-plugin", "mode=" + orDefaultStr(mode, "websocket")}
		if host != "" {
			parts = append(parts, "host="+host)
		}
		if v, ok := opts["tls"].(bool); ok && v {
			parts = append(parts, "tls")
		}
		if path, ok := opts["path"].(string); ok && path != "" {
			parts = append(parts, "path="+path)
		}
		return "plugin=" + url.QueryEscape(strings.Join(parts, ";"))
	}
	return ""
}

func fillTransport(v map[string]string, p model.Proxy) {
	switch p.String("network") {
	case "ws":
		path, host := wsOpts(p)
		v["path"] = path
		v["host"] = host
		v["type"] = "none"
	case "grpc":
		v["net"] = "grpc"
		if o, ok := p["grpc-opts"].(map[string]any); ok {
			if s, ok := o["grpc-service-name"].(string); ok {
				v["path"] = s
			}
		}
	case "h2":
		v["net"] = "h2"
		v["type"] = "http"
		if o, ok := p["h2-opts"].(map[string]any); ok {
			if path, ok := o["path"].(string); ok {
				v["path"] = path
			}
			if hosts, ok := o["host"].([]any); ok && len(hosts) > 0 {
				if h, ok := hosts[0].(string); ok {
					v["host"] = h
				}
			} else if h, ok := o["host"].(string); ok {
				v["host"] = h
			}
		}
	default:
		v["type"] = "none"
	}
}

func setTLSQuery(q url.Values, p model.Proxy) {
	if o, ok := p["reality-opts"].(map[string]any); ok {
		q.Set("security", "reality")
		if pbk, ok := o["public-key"].(string); ok && pbk != "" {
			q.Set("pbk", pbk)
		}
		if sid, ok := o["short-id"].(string); ok && sid != "" {
			q.Set("sid", sid)
		}
		if fp := p.String("client-fingerprint"); fp != "" {
			q.Set("fp", fp)
		}
	} else if p.Bool("tls") {
		q.Set("security", "tls")
	} else {
		q.Set("security", "none")
	}
	if sni := sniOf(p); sni != "" {
		q.Set("sni", sni)
	}
}

func setTransportQuery(q url.Values, p model.Proxy) {
	switch p.String("network") {
	case "ws":
		path, host := wsOpts(p)
		q.Set("type", "ws")
		q.Set("path", path)
		if host != "" {
			q.Set("host", host)
		}
	case "grpc":
		q.Set("type", "grpc")
		if o, ok := p["grpc-opts"].(map[string]any); ok {
			if s, ok := o["grpc-service-name"].(string); ok && s != "" {
				q.Set("serviceName", s)
			}
		}
	case "h2":
		q.Set("type", "http")
	default:
		q.Set("type", "tcp")
	}
}

func alpnOf(p model.Proxy) []string {
	switch v := p["alpn"].(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	case []string:
		if len(v) > 0 {
			return v
		}
	}
	return []string{"h3"}
}
