package scheme

import (
	"strings"
	"testing"

	"towerdocker/internal/model"
)

func TestUpstreamGroupOrder(t *testing.T) {
	st := &model.State{}
	if err := Apply(st, PresetFull); err != nil {
		t.Fatalf("应用方案失败：%v", err)
	}
	want := []string{
		"节点选择", "手动切换", "自动选择",
		"🚀 节点选择", "🎛️ 手动切换", "♻️ 自动选择", "🎯 直接连接",
		"国外广告", "Google FCM", "Microsoft", "苹果服务", "Telegram",
		"AI服务", "国内流量", "国际流量", "YouTube", "国外媒体",
	}
	got := make([]string, 0, len(st.Groups))
	for _, g := range st.Groups {
		got = append(got, g.Name)
	}
	if len(got) != len(want) {
		t.Fatalf("组数不对：期望 %d，实际 %d（%v）", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 个组应为 %s，实际 %s", i+1, want[i], got[i])
		}
	}
	if !st.Groups[3].Hidden || !st.Groups[6].Hidden {
		t.Errorf("嵌套组应为 hidden")
	}
}

func TestUpstreamRuleOrder(t *testing.T) {
	st := &model.State{}
	if err := Apply(st, PresetFull); err != nil {
		t.Fatalf("应用方案失败：%v", err)
	}
	rules := st.OrderedRules()
	if len(rules) != len(acl4ssrSourcesFull)+1 {
		t.Fatalf("规则数不对：期望 %d，实际 %d", len(acl4ssrSourcesFull)+1, len(rules))
	}
	if rules[0].Value != "LocalAreaNetwork" || rules[0].Type != "RULE-SET" {
		t.Errorf("第一条规则应为 LocalAreaNetwork，实际 %v", rules[0])
	}
	if rules[1].Value != "UnBan" || rules[4].Value != "GoogleFCM" {
		t.Errorf("规则顺序与 ACL4SSR 不一致：%s / %s", rules[1].Value, rules[4].Value)
	}
	if last := rules[len(rules)-1]; last.Type != "GEOIP" || last.Value != "CN" || last.Params != "no-resolve" {
		t.Errorf("最后一条应为 GEOIP,CN,no-resolve，实际 %v", last)
	}
	if st.FinalTarget() != "国际流量" {
		t.Errorf("MATCH 应落到国际流量，实际 %s", st.FinalTarget())
	}
	// 广告类规则落到「国外广告」，直连类落到 DIRECT
	if byName := findRule(rules, "BanAD"); byName == nil || byName.Target != "国外广告" {
		t.Errorf("BanAD 应落到国外广告，实际 %v", byName)
	}
	if byName := findRule(rules, "ChinaDomain"); byName == nil || byName.Target != "DIRECT" {
		t.Errorf("ChinaDomain 应落到 DIRECT，实际 %v", byName)
	}
}

func findRule(rules []model.Rule, value string) *model.Rule {
	for i := range rules {
		if rules[i].Value == value {
			return &rules[i]
		}
	}
	return nil
}

// 三套规则源表都必须与对应的 ACL4SSR ini 对齐：源数量、首末源、规则目标。
func TestPresetRuleTables(t *testing.T) {
	cases := []struct {
		id       string
		sources  int
		lastSrc  string
		wantPols []string
	}{
		{PresetFull, 31, "Download", []string{
			"国外广告", "Google FCM", "Microsoft", "苹果服务", "Telegram",
			"AI服务", "国内流量", "国际流量", "YouTube", "国外媒体",
		}},
		{PresetDefault, 14, "ChinaCompanyIp", []string{
			"国外广告", "Google FCM", "Microsoft", "苹果服务", "Telegram", "国外媒体", "国际流量",
		}},
		{PresetMini, 11, "ChinaCompanyIp", []string{"国外广告", "国际流量"}},
	}
	for _, c := range cases {
		st := &model.State{}
		if err := Apply(st, c.id); err != nil {
			t.Fatalf("%s 应用失败：%v", c.id, err)
		}
		rules := st.OrderedRules()
		if len(rules) != c.sources+1 {
			t.Errorf("%s 规则数应为 %d，实际 %d", c.id, c.sources+1, len(rules))
		}
		if rules[0].Value != "LocalAreaNetwork" || rules[len(rules)-2].Value != c.lastSrc {
			t.Errorf("%s 规则源首末不对：%s / %s", c.id, rules[0].Value, rules[len(rules)-2].Value)
		}
		for i, r := range st.Rules {
			if r.Seq != i+1 {
				t.Errorf("%s 第 %d 条规则 seq 应为 %d，实际 %d", c.id, i+1, i+1, r.Seq)
				break
			}
		}
		if len(st.RuleSets) != c.sources+1 {
			t.Errorf("%s rule-provider 数应为 %d，实际 %d", c.id, c.sources+1, len(st.RuleSets))
		}
		// 默认预置 antiad 规则集，但没有任何规则引用它
		found := false
		for _, rs := range st.RuleSets {
			if rs.Name == antiadRuleSet {
				found = true
			}
		}
		if !found {
			t.Errorf("%s 缺少默认规则集 %s", c.id, antiadRuleSet)
		}
		for _, r := range st.Rules {
			if r.Type == "RULE-SET" && r.Value == antiadRuleSet {
				t.Errorf("%s 不应该引用 %s", c.id, antiadRuleSet)
			}
		}
		// rule-provider URL 必须钉在当前 revision 上，构建注入后自动跟着变
		for _, rs := range st.RuleSets {
			if !strings.Contains(rs.URL, "/"+ACL4SSRRevision+"/") {
				t.Errorf("%s 规则集 %s 的 URL 未带 revision %s：%s", c.id, rs.Name, ACL4SSRRevision, rs.URL)
				break
			}
		}
		// 可见组 = 3 个主组 + 规则策略组，策略组顺序按规则源首次出现
		visible := make([]string, 0, len(st.Groups))
		for _, g := range st.Groups {
			if !g.Hidden {
				visible = append(visible, g.Name)
			}
		}
		want := append([]string{"节点选择", "手动切换", "自动选择"}, c.wantPols...)
		if strings.Join(visible, "|") != strings.Join(want, "|") {
			t.Errorf("%s 可见组顺序不对：\n实际 %v\n期望 %v", c.id, visible, want)
		}
		if st.FinalTarget() != "国际流量" {
			t.Errorf("%s MATCH 应落到国际流量，实际 %s", c.id, st.FinalTarget())
		}
		// 规则目标与兜底策略都必须指向真实存在的组，否则客户端加载会报 proxy not found
		names := map[string]bool{}
		for _, g := range st.Groups {
			names[g.Name] = true
		}
		if !names[st.FinalTarget()] {
			t.Errorf("%s 兜底策略 %s 没有对应组", c.id, st.FinalTarget())
		}
		for _, r := range st.Rules {
			if r.Target != policyDirect && !names[r.Target] {
				t.Errorf("%s 规则 %s 指向不存在的组 %s", c.id, r.Value, r.Target)
			}
		}
	}
	// Mini 里电报/国外媒体/国际代理都走「节点选择」
	st := &model.State{}
	_ = Apply(st, PresetMini)
	for _, name := range []string{"Telegram", "ProxyMedia", "ProxyLite"} {
		if r := findRule(st.OrderedRules(), name); r == nil || r.Target != policySelect {
			t.Errorf("Mini 里 %s 应落到节点选择，实际 %v", name, r)
		}
	}
}

func TestSyncRegions(t *testing.T) {
	st := &model.State{}
	if err := Apply(st, PresetFull); err != nil {
		t.Fatal(err)
	}
	st.Subs = []model.Subscription{{Nodes: []model.Proxy{
		{"name": "🇭🇰 香港 01", "type": "ss"},
		{"name": "HK-02", "type": "ss"},
		{"name": "🇯🇵 日本 IPLC", "type": "ss"},
		{"name": "美国 洛杉矶", "type": "ss"},
		{"name": "🇸🇪 瑞典", "type": "ss"},
		{"name": "企业专线", "type": "ss"},
	}}}
	SyncRegions(st)

	names := map[string]model.Group{}
	for _, g := range st.Groups {
		names[g.Name] = g
	}
	hk := names["🇭🇰 香港"]
	if hk.Name == "" {
		t.Fatalf("缺少香港组：%v", names)
	}
	if hk.Members[0] != "🇭🇰 香港"+RegionAutoSuffix || len(hk.Members) != 3 {
		t.Errorf("香港组成员顺序不对：%v", hk.Members)
	}
	if auto := names["🇭🇰 香港"+RegionAutoSuffix]; auto.Type != "url-test" || !auto.Hidden {
		t.Errorf("地区测速组应为 hidden url-test：%v", auto)
	}
	if _, ok := names[OtherRegionsName]; !ok {
		t.Errorf("国旗不在 9 个地区内的节点应进其他地区组")
	}
	if _, ok := names["🇺🇸 美国"]; !ok {
		t.Errorf("缺少美国组")
	}
	// 地区组必须排在策略组之后
	if last := st.Groups[len(st.Groups)-1].Name; last != OtherRegionsName && last != OtherRegionsName+RegionAutoSuffix {
		t.Errorf("地区组应追加在最后，实际结尾是 %s", last)
	}

	// 再次同步必须幂等：「其他地区」的自动测速组也要一起清掉，否则会重复堆积。
	SyncRegions(st)
	seen := map[string]bool{}
	for _, g := range st.Groups {
		if seen[g.Name] {
			t.Fatalf("重复的组名：%s", g.Name)
		}
		seen[g.Name] = true
	}
}

func TestDetectRegion(t *testing.T) {
	cases := map[string]string{
		"🇭🇰 香港 01":       "HK",
		"HK-02":          "HK",
		"🇯🇵 日本 IPLC":     "JP",
		"美国 洛杉矶":         "US",
		"🇸🇬 狮城":          "SG",
		"台湾 新北":          "TW",
		"韩国 首尔":          "KR",
		"🇸🇪 瑞典":          OtherRegionsName,
		"企业专线":           "",
		"My Node":        "", // MY 是马来西亚码，但小写不算
		"Contact us":     "",
		"US Premium":     "US",
		"Netherlands DE": "DE",
	}
	for name, want := range cases {
		if got := DetectRegion(name); got != want {
			t.Errorf("DetectRegion(%q)=%q，期望 %q", name, got, want)
		}
	}
}
