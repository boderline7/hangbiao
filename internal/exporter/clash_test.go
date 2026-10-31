package exporter

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"towerdocker/internal/model"
)

func clashTestState(t *testing.T, s model.Settings) *model.State {
	t.Helper()
	st := &model.State{
		Subs: []model.Subscription{{ID: "s1", Name: "测试", Nodes: []model.Proxy{{
			"name": "🇭🇰 香港", "type": "ss", "server": "1.2.3.4", "port": 8388,
			"cipher": "aes-128-gcm", "password": "pw",
		}}}},
		Groups: []model.Group{{ID: "g1", Name: "节点选择", Type: "select", Members: []string{"$nodes", "DIRECT"}}},
		Rules: []model.Rule{{
			ID: "r1", Type: "DOMAIN-SUFFIX", Value: "example.com", Target: "节点选择", Position: "g1",
		}},
		Settings: s,
	}
	st.Settings.Title = "航标"
	st.Settings.MixedPort = 7890
	st.Normalize()
	return st
}

func renderClash(t *testing.T, st *model.State) map[string]any {
	t.Helper()
	res, err := Clash(st)
	if err != nil {
		t.Fatalf("Clash 失败：%v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(res.Body, &doc); err != nil {
		t.Fatalf("YAML 解析失败：%v", err)
	}
	return doc
}

func rulesOf(t *testing.T, doc map[string]any) []string {
	t.Helper()
	raw, ok := doc["rules"].([]any)
	if !ok {
		t.Fatalf("rules 类型不对：%T", doc["rules"])
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(string))
	}
	return out
}

func TestClashDefaults(t *testing.T) {
	doc := renderClash(t, clashTestState(t, model.Settings{}))

	if doc["ipv6"] != true {
		t.Errorf("默认应开启 ipv6，实际 %v", doc["ipv6"])
	}

	tun, ok := doc["tun"].(map[string]any)
	if !ok {
		t.Fatal("默认应该写 tun 段")
	}
	if tun["enable"] != true {
		t.Errorf("tun.enable=%v", tun["enable"])
	}
	ex, _ := tun["route-exclude-address"].([]any)
	if len(ex) != 2 {
		t.Fatalf("route-exclude-address=%v", tun["route-exclude-address"])
	}
	if ex[0] != "100.64.0.0/10" || ex[1] != "fd7a:115c:a1e0::/48" {
		t.Errorf("route-exclude-address=%v", ex)
	}

	dns, ok := doc["dns"].(map[string]any)
	if !ok {
		t.Fatal("默认应该写 dns 段")
	}
	if dns["ipv6"] != true {
		t.Errorf("dns.ipv6 应跟随总开关，实际 %v", dns["ipv6"])
	}
	for _, key := range []string{"nameserver", "fallback"} {
		list, _ := dns[key].([]any)
		if len(list) == 0 {
			t.Fatalf("%s 为空", key)
		}
		for _, v := range list {
			if !strings.HasPrefix(v.(string), "https://") {
				t.Errorf("%s 不是加密 DNS：%v", key, v)
			}
		}
	}

	rules := rulesOf(t, doc)
	wantBypass := bypassRules(model.Settings{})
	if len(rules) < len(wantBypass)+1 {
		t.Fatalf("rules=%v", rules)
	}
	for i, w := range wantBypass {
		if rules[i] != w {
			t.Errorf("rules[%d]=%q，期望 %q", i, rules[i], w)
		}
	}
	if rules[len(wantBypass)] != "DOMAIN-SUFFIX,example.com,节点选择" {
		t.Errorf("直连规则之后的第一个规则应为分组规则，实际 %q", rules[len(wantBypass)])
	}
	if rules[len(rules)-1] != "MATCH,节点选择" {
		t.Errorf("最后一条应为 MATCH，实际 %q", rules[len(rules)-1])
	}

	// BT 开关默认覆盖常见客户端：域名 + 各平台进程名。
	has := func(line string) bool {
		for _, r := range rules {
			if r == line {
				return true
			}
		}
		return false
	}
	for _, line := range []string{
		"DOMAIN-SUFFIX,qbittorrent.org,DIRECT",
		"DOMAIN-SUFFIX,transmissionbt.com,DIRECT",
		"PROCESS-NAME,transmission-daemon,DIRECT",
		"PROCESS-NAME,qBittorrent,DIRECT",
		"PROCESS-NAME,BitComet,DIRECT",
	} {
		if !has(line) {
			t.Errorf("缺少 BT 直连规则 %q", line)
		}
	}
}

func TestClashIPv6Disabled(t *testing.T) {
	doc := renderClash(t, clashTestState(t, model.Settings{IPv6Disabled: true}))

	if doc["ipv6"] != false {
		t.Errorf("关闭 IPv6 后顶层 ipv6=%v", doc["ipv6"])
	}
	dns, ok := doc["dns"].(map[string]any)
	if !ok {
		t.Fatal("默认应该写 dns 段")
	}
	if dns["ipv6"] != false {
		t.Errorf("关闭 IPv6 后 dns.ipv6=%v", dns["ipv6"])
	}
}

func TestClashDisableSwitches(t *testing.T) {
	doc := renderClash(t, clashTestState(t, model.Settings{
		DisableDNS: true, TunDisabled: true, TailscaleDisabled: true,
		LocalIPDisabled: true, VerysyncDisabled: true, BtDisabled: true,
		CommonOnlyDisabled: true,
	}))

	if _, ok := doc["tun"]; ok {
		t.Error("关闭 TUN 后不应该写 tun 段")
	}
	if _, ok := doc["dns"]; ok {
		t.Error("关闭加密 DNS 后不应该写 dns 段")
	}
	rules := rulesOf(t, doc)
	if rules[0] != "DOMAIN-SUFFIX,example.com,节点选择" {
		t.Errorf("rules[0]=%q", rules[0])
	}
}
