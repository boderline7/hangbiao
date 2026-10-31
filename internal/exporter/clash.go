// Package exporter 把内部模型渲染成各客户端配置。
//
// 规则顺序完全由策略组顺序决定：先按组的先后取出各组的规则，最后补一条 MATCH。
package exporter

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"towerdocker/internal/model"
	"towerdocker/internal/scheme"
)

type Result struct {
	Body     []byte
	Filename string
	Warnings []string
	// Kept 是在目标客户端里成功转换的节点数，0 表示这份配置没有可用节点。
	Kept int
}

var ErrUnknownFormat = errors.New("不支持的导出格式")

var tailscaleCIDRs = []string{"100.64.0.0/10", "fd7a:115c:a1e0::/48"}

// bypassRules 生成放在所有分组规则之前的直连/旁路规则。
// 顺序：Tailscale → 本地 IP → 微力同步 → BT 下载 → 仅代理常见端口。
func bypassRules(s model.Settings) []string {
	var out []string
	if !s.TailscaleDisabled {
		out = append(out, "IP-CIDR,100.64.0.0/10,DIRECT,no-resolve")
		out = append(out, "IP-CIDR6,fd7a:115c:a1e0::/48,DIRECT,no-resolve")
	}
	if !s.LocalIPDisabled {
		out = append(out,
			"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
			"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
			"IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
			"IP-CIDR,127.0.0.1/8,DIRECT,no-resolve",
			"IP-CIDR6,fe80::/10,DIRECT,no-resolve",
			"IP-CIDR6,fc00::/7,DIRECT,no-resolve",
		)
	}
	if !s.VerysyncDisabled {
		out = append(out,
			"DOMAIN-SUFFIX,verysync.com,DIRECT",
			"PROCESS-NAME,verysync,DIRECT",
			"PROCESS-NAME,Verysync,DIRECT",
		)
	}
	if !s.BtDisabled {
		// 常见 BT 客户端：域名走官网/检查更新，进程名覆盖 Linux、macOS、Windows 的写法。
		out = append(out,
			"DOMAIN-SUFFIX,qbittorrent.org,DIRECT",
			"DOMAIN-SUFFIX,qbittorrent.com,DIRECT",
			"DOMAIN-SUFFIX,transmissionbt.com,DIRECT",
			"DOMAIN-SUFFIX,deluge-torrent.org,DIRECT",
			"DOMAIN-SUFFIX,utorrent.com,DIRECT",
			"DOMAIN-SUFFIX,vuze.com,DIRECT",
			"PROCESS-NAME,qbittorrent,DIRECT",
			"PROCESS-NAME,qbittorrent-nox,DIRECT",
			"PROCESS-NAME,qBittorrent,DIRECT",
			"PROCESS-NAME,transmission,DIRECT",
			"PROCESS-NAME,transmission-daemon,DIRECT",
			"PROCESS-NAME,transmission-gtk,DIRECT",
			"PROCESS-NAME,Transmission,DIRECT",
			"PROCESS-NAME,deluge,DIRECT",
			"PROCESS-NAME,deluged,DIRECT",
			"PROCESS-NAME,Deluge,DIRECT",
			"PROCESS-NAME,aria2c,DIRECT",
			"PROCESS-NAME,aria2,DIRECT",
			"PROCESS-NAME,rtorrent,DIRECT",
			"PROCESS-NAME,utorrent,DIRECT",
			"PROCESS-NAME,uTorrent,DIRECT",
			"PROCESS-NAME,BitComet,DIRECT",
			"PROCESS-NAME,vuze,DIRECT",
			"PROCESS-NAME,Vuze,DIRECT",
			"PROCESS-NAME,tixati,DIRECT",
			"PROCESS-NAME,Tixati,DIRECT",
			"PROCESS-NAME,biglybt,DIRECT",
			"PROCESS-NAME,BiglyBT,DIRECT",
		)
	}
	if !s.CommonOnlyDisabled {
		// 非常见端口一律直连：只有 80/443/8080/8443 才交给后面的分流规则。
		out = append(out,
			"NOT,((OR,((DST-PORT,80),(DST-PORT,443),(DST-PORT,8080),(DST-PORT,8443)))),DIRECT",
		)
	}
	return out
}

func Build(st *model.State, format string) (Result, error) {
	switch format {
	case "", "clash", "yaml", "auto":
		r, err := Clash(st)
		if err != nil {
			return Result{}, err
		}
		r.Filename = safeName(st.Settings.Title) + ".yaml"
		return r, nil
	case "passwall", "pw":
		return annotate(PassWall(st), st, "PassWall"), nil
	}
	return Result{}, ErrUnknownFormat
}

// ---------- 成员展开 ----------

var builtinTargets = map[string]bool{
	"DIRECT": true, "REJECT": true, "REJECT-DROP": true,
	"PASS": true, "COMPATIBLE": true, "GLOBAL": true,
}

type refs struct {
	nodes  []string
	groups []string
	// regions 是地区组（🇭🇰 香港 之类），供 $regions 占位符展开。
	regions []string
}

func collectRefs(st *model.State) refs {
	r := refs{}
	for _, p := range st.Nodes() {
		r.nodes = append(r.nodes, p.Name())
	}
	for _, g := range st.Groups {
		r.groups = append(r.groups, g.Name)
	}
	for _, name := range scheme.RegionGroupNames(st) {
		r.regions = append(r.regions, name)
	}
	return r
}

// expandMembers 把 $nodes / $groups 展开，并丢掉不存在的引用与自引用。
func expandMembers(g model.Group, r refs) []string {
	knownNode := map[string]bool{}
	for _, n := range r.nodes {
		knownNode[n] = true
	}
	knownGroup := map[string]bool{}
	for _, n := range r.groups {
		knownGroup[n] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(g.Members))
	add := func(name string) {
		if name == "" || name == g.Name || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, m := range g.Members {
		switch m {
		case "$nodes":
			for _, n := range r.nodes {
				add(n)
			}
		case "$groups":
			for _, n := range r.groups {
				add(n)
			}
		case "$regions":
			for _, n := range r.regions {
				add(n)
			}
		default:
			if builtinTargets[m] || knownNode[m] || knownGroup[m] {
				add(m)
			}
		}
	}
	if len(out) == 0 {
		out = append(out, "DIRECT")
	}
	return out
}

// ---------- YAML 节点工具 ----------

func scalar(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

func toNode(v any) (*yaml.Node, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		return nil, err
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil, nil
		}
		return n.Content[0], nil
	}
	return &n, nil
}

func appendKV(m *yaml.Node, k string, v any) {
	n, err := toNode(v)
	if err != nil || n == nil {
		return
	}
	m.Content = append(m.Content, scalar(k), n)
}

func appendNode(m *yaml.Node, k string, n *yaml.Node) {
	if n == nil {
		return
	}
	m.Content = append(m.Content, scalar(k), n)
}

var proxyKeyOrder = []string{
	"name", "type", "server", "port", "cipher", "password", "uuid", "alterId",
	"udp", "tls", "sni", "servername", "network", "ws-opts", "grpc-opts",
	"h2-opts", "flow", "reality-opts", "client-fingerprint", "skip-cert-verify",
	"plugin", "plugin-opts",
}

func proxyNode(p model.Proxy) (*yaml.Node, error) {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	used := map[string]bool{}
	for _, k := range proxyKeyOrder {
		v, ok := p[k]
		if !ok {
			continue
		}
		used[k] = true
		appendKV(n, k, v)
	}
	rest := make([]string, 0, len(p))
	for k := range p {
		if !used[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		appendKV(n, k, p[k])
	}
	return n, nil
}

// ---------- Clash / mihomo ----------

func Clash(st *model.State) (Result, error) {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	ref := collectRefs(st)

	appendKV(root, "mixed-port", st.Settings.MixedPort)
	appendKV(root, "allow-lan", st.Settings.AllowLAN)
	appendKV(root, "mode", st.Settings.Mode)
	appendKV(root, "log-level", st.Settings.LogLevel)
	appendKV(root, "ipv6", !st.Settings.IPv6Disabled)
	appendKV(root, "unified-delay", true)
	appendKV(root, "tcp-concurrent", true)
	if st.Settings.ExternalController != "" {
		appendKV(root, "external-controller", st.Settings.ExternalController)
	}
	if st.Settings.Secret != "" {
		appendKV(root, "secret", st.Settings.Secret)
	}

	if !st.Settings.TunDisabled {
		tun := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		appendKV(tun, "enable", true)
		appendKV(tun, "stack", "system")
		appendKV(tun, "dns-hijack", []string{"any:53"})
		appendKV(tun, "auto-route", true)
		appendKV(tun, "auto-detect-interface", true)
		if !st.Settings.TailscaleDisabled {
			appendKV(tun, "route-exclude-address", tailscaleCIDRs)
		}
		appendNode(root, "tun", tun)
	}

	if !st.Settings.DisableDNS {
		dns := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		appendKV(dns, "enable", true)
		appendKV(dns, "ipv6", !st.Settings.IPv6Disabled)
		appendKV(dns, "enhanced-mode", "fake-ip")
		appendKV(dns, "fake-ip-range", "198.18.0.1/16")
		appendKV(dns, "default-nameserver", []string{"223.5.5.5", "119.29.29.29"})
		appendKV(dns, "nameserver", []string{"https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"})
		appendKV(dns, "fallback", []string{"https://1.1.1.1/dns-query", "https://dns.google/dns-query"})
		appendNode(root, "dns", dns)
	}

	pseq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, p := range st.Nodes() {
		n, err := proxyNode(p)
		if err != nil {
			return Result{}, err
		}
		pseq.Content = append(pseq.Content, n)
	}
	appendNode(root, "proxies", pseq)

	gseq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, g := range st.Groups {
		gn := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		appendKV(gn, "name", g.Name)
		appendKV(gn, "type", g.Type)
		appendKV(gn, "proxies", expandMembers(g, ref))
		if g.Hidden {
			appendKV(gn, "hidden", true)
		}
		if g.Type == "url-test" || g.Type == "fallback" || g.Type == "load-balance" {
			url := g.URL
			if url == "" {
				url = "http://www.gstatic.com/generate_204"
			}
			appendKV(gn, "url", url)
			interval := g.Interval
			if interval == 0 {
				interval = 300
			}
			appendKV(gn, "interval", interval)
			appendKV(gn, "tolerance", 50)
			appendKV(gn, "lazy", true)
		}
		gseq.Content = append(gseq.Content, gn)
	}
	appendNode(root, "proxy-groups", gseq)

	used := st.UsedRuleSets()
	rp := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, rs := range st.RuleSets {
		if !used[rs.Name] {
			continue
		}
		inner := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		appendKV(inner, "type", "http")
		appendKV(inner, "behavior", rs.Behavior)
		if rs.Format != "" {
			appendKV(inner, "format", rs.Format)
		}
		appendKV(inner, "url", rs.URL)
		appendKV(inner, "path", rs.Path)
		appendKV(inner, "interval", rs.Interval)
		appendNode(rp, rs.Name, inner)
	}
	if len(rp.Content) > 0 {
		appendNode(root, "rule-providers", rp)
	}

	rseq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	// 直连/旁路规则放在分组规则之前，优先级最高。
	for _, line := range bypassRules(st.Settings) {
		rseq.Content = append(rseq.Content, scalar(line))
	}
	for _, r := range st.OrderedRules() {
		rseq.Content = append(rseq.Content, scalar(r.Line()))
	}
	rseq.Content = append(rseq.Content, scalar("MATCH,"+st.FinalTarget()))
	appendNode(root, "rules", rseq)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return Result{}, err
	}
	if err := enc.Close(); err != nil {
		return Result{}, err
	}

	res := Result{Body: buf.Bytes(), Kept: len(st.Nodes())}
	if len(st.Nodes()) == 0 {
		res.Warnings = append(res.Warnings, "当前没有任何节点，proxies 为空，客户端会加载失败")
	}
	return res, nil
}

// annotate 在「一个节点都没转换成功」时给出显式警告，避免产出看起来正常、
// 实际不可用的配置文件。
func annotate(r Result, st *model.State, client string) Result {
	if r.Kept == 0 && len(st.Nodes()) > 0 {
		msg := fmt.Sprintf("没有任何节点能转换成 %s 格式（当前订阅的协议它不支持），这份配置不可用；请改用 Clash，或换支持这些协议的客户端", client)
		r.Warnings = append([]string{msg}, r.Warnings...)
	}
	return r
}

func safeName(s string) string {
	if s == "" {
		return "config"
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			out = append(out, '-')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

func header(st *model.State, client string) string {
	return fmt.Sprintf("# %s · %s 配置\n# 由 航标 生成，策略组顺序即规则优先级\n# 节点 %d 个 / 组 %d 个 / 规则 %d 条\n\n",
		st.Settings.Title, client, len(st.Nodes()), len(st.Groups), len(st.OrderedRules())+1)
}
