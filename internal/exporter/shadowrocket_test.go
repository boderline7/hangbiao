package exporter

import (
	"bytes"
	"testing"

	"gopkg.in/yaml.v3"

	"towerdocker/internal/model"
)

func shadowrocketTestState(t *testing.T, s model.Settings) *model.State {
	t.Helper()
	st := &model.State{
		Subs: []model.Subscription{{ID: "s1", Name: "测试", Nodes: []model.Proxy{{
			"name": "🇭🇰 香港", "type": "ss", "server": "1.2.3.4", "port": 8388,
			"cipher": "aes-128-gcm", "password": "pw",
		}}}},
		Groups: []model.Group{
			{ID: "g1", Name: "节点选择", Type: "select", Members: []string{"$nodes", "自动选择", "DIRECT"}},
			{ID: "g2", Name: "自动选择", Type: "url-test", Members: []string{"$nodes"}},
		},
		Rules: []model.Rule{{
			ID: "r1", Type: "RULE-SET", Value: "广告", Target: "REJECT", Position: "g1",
		}},
		RuleSets: []model.RuleSet{{
			ID: "rs1", Name: "广告", Behavior: "domain", Format: "text",
			URL: "https://example.com/ad.list", Interval: 86400,
		}},
		Settings: s,
	}
	st.Settings.Title = "航标"
	st.Settings.MixedPort = 7890
	st.Settings.AllowLAN = true
	st.Normalize()
	return st
}

func renderShadowrocket(t *testing.T, st *model.State) map[string]any {
	t.Helper()
	res, err := Shadowrocket(st)
	if err != nil {
		t.Fatalf("Shadowrocket 失败：%v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(res.Body, &doc); err != nil {
		t.Fatalf("YAML 解析失败：%v", err)
	}
	return doc
}

func TestShadowrocketDropsMihomoKeys(t *testing.T) {
	doc := renderShadowrocket(t, shadowrocketTestState(t, model.Settings{TunEnabled: true}))

	for _, key := range []string{"tun", "mixed-port", "allow-lan", "unified-delay", "tcp-concurrent"} {
		if _, ok := doc[key]; ok {
			t.Errorf("小火箭导出不应包含 %q", key)
		}
	}

	groups, ok := doc["proxy-groups"].([]any)
	if !ok {
		t.Fatalf("proxy-groups 类型不对：%T", doc["proxy-groups"])
	}
	for _, raw := range groups {
		g, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("proxy-groups 元素类型不对：%T", raw)
		}
		for _, key := range []string{"default-selected", "lazy"} {
			if _, ok := g[key]; ok {
				t.Errorf("proxy-groups[%v] 不应包含 %q", g["name"], key)
			}
		}
	}

	for _, key := range []string{"dns", "proxies", "proxy-groups", "rule-providers", "rules"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("小火箭导出缺少 %q", key)
		}
	}
}

func TestShadowrocketIgnoresTun(t *testing.T) {
	off, err := Shadowrocket(shadowrocketTestState(t, model.Settings{}))
	if err != nil {
		t.Fatalf("Shadowrocket 失败：%v", err)
	}
	on, err := Shadowrocket(shadowrocketTestState(t, model.Settings{TunEnabled: true}))
	if err != nil {
		t.Fatalf("Shadowrocket 失败：%v", err)
	}
	if !bytes.Equal(off.Body, on.Body) {
		t.Error("TUN 开关不应影响小火箭导出内容")
	}
}

func TestShadowrocketFilename(t *testing.T) {
	st := shadowrocketTestState(t, model.Settings{})
	for _, format := range []string{"shadowrocket", "sr"} {
		res, err := Build(st, format)
		if err != nil {
			t.Fatalf("Build(%q) 失败：%v", format, err)
		}
		if res.Filename != "航标-shadowrocket.yaml" {
			t.Errorf("Build(%q) 文件名=%q", format, res.Filename)
		}
	}
}
