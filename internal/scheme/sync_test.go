package scheme

import (
	"strings"
	"testing"

	"towerdocker/internal/model"
)

const sampleIni = `[custom]
; 注释与空行都要忽略
ruleset=🎯 全球直连,https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/LocalAreaNetwork.list
ruleset=🛑 广告拦截,https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/Ruleset/BanAD.list
ruleset=🎯 全球直连,[]GEOIP,CN
ruleset=🎯 全球直连,[]DOMAIN-SUFFIX,xn--ngstr-lra8j.com
ruleset=🐟 漏网之鱼,[]FINAL

custom_proxy_group=🚀 节点选择` + "`select`" + `[]♻️ 自动选择` + "`[]DIRECT`" + `.*
custom_proxy_group=♻️ 自动选择` + "`url-test`" + `.*` + "`http://www.gstatic.com/generate_204`" + `300,,50
custom_proxy_group=🎯 全球直连` + "`select`" + `[]DIRECT` + "`[]🚀 节点选择`" + `[]♻️ 自动选择
custom_proxy_group=🇭🇰 香港节点` + "`url-test`" + `(港|HK|hk|Hong Kong)` + "`http://www.gstatic.com/generate_204`" + `300,,50
custom_proxy_group=🛑 广告拦截` + "`select`" + `[]REJECT` + "`[]DIRECT" + `
custom_proxy_group=🐟 漏网之鱼` + "`select`" + `[]🚀 节点选择` + "`[]DIRECT`" + `.*
`

func TestParseIni(t *testing.T) {
	def, warnings := parseIni(sampleIni)
	if len(warnings) != 0 {
		t.Errorf("不该有解析告警：%v", warnings)
	}
	if len(def.Sources) != 3 {
		t.Fatalf("应有 2 个规则源 + 1 条内联规则，实际 %d", len(def.Sources))
	}
	if s := def.Sources[0]; s.Name != "LocalAreaNetwork" || s.Path != "LocalAreaNetwork.list" || s.Policy != "🎯 全球直连" {
		t.Errorf("第一个规则源不对：%+v", s)
	}
	if s := def.Sources[1]; s.Name != "BanAD" || s.Path != "Ruleset/BanAD.list" || s.Policy != "🛑 广告拦截" {
		t.Errorf("第二个规则源不对：%+v", s)
	}
	if s := def.Sources[2]; s.RuleType != "DOMAIN-SUFFIX" || s.RuleValue != "xn--ngstr-lra8j.com" || s.Policy != "🎯 全球直连" {
		t.Errorf("内联规则不对：%+v", s)
	}
	if def.GeoIP != "CN" || def.GeoIPTarget != "🎯 全球直连" {
		t.Errorf("GEOIP 解析不对：%q / %q", def.GeoIP, def.GeoIPTarget)
	}
	if def.Final != "🐟 漏网之鱼" {
		t.Errorf("FINAL 解析不对：%q", def.Final)
	}

	if len(def.Groups) != 6 {
		t.Fatalf("应有 6 个策略组，实际 %d：%+v", len(def.Groups), def.Groups)
	}
	if g := def.Groups[0]; g.Name != "🚀 节点选择" || g.Type != "select" ||
		len(g.Choices) != 2 || len(g.Filters) != 1 || g.Filters[0] != ".*" {
		t.Errorf("节点选择组不对：%+v", g)
	}
	if g := def.Groups[1]; g.Type != "url-test" || g.Interval != 300 ||
		g.URL != "http://www.gstatic.com/generate_204" {
		t.Errorf("自动选择组不对：%+v", g)
	}
	if g := def.Groups[2]; g.Name != "🎯 全球直连" || len(g.Choices) != 3 {
		t.Errorf("全球直连组不对：%+v", g)
	}
	if g := def.Groups[3]; g.Name != "🇭🇰 香港节点" || g.Filters[0] != "(港|HK|hk|Hong Kong)" {
		t.Errorf("地区组不对：%+v", g)
	}
	if g := def.Groups[4]; g.Name != "🛑 广告拦截" || len(g.Choices) != 2 || g.Choices[1] != "DIRECT" {
		t.Errorf("广告拦截组不对：%+v", g)
	}
}

func TestApplyUpstream(t *testing.T) {
	def, _ := parseIni(sampleIni)
	st := &model.State{
		Subs: []model.Subscription{{ID: "s1", Nodes: []model.Proxy{
			{"name": "🇭🇰 香港 01"}, {"name": "🇯🇵 JP-01"}, {"name": "US Node"},
		}}},
		Settings: model.Settings{Title: "航标"},
	}
	warnings := applyUpstream(st, def)
	if len(warnings) != 0 {
		t.Errorf("不该有构建告警：%v", warnings)
	}

	byName := map[string]model.Group{}
	for _, g := range st.Groups {
		byName[g.Name] = g
	}
	selectGroup := byName["🚀 节点选择"]
	if selectGroup.Type != "select" || selectGroup.Members[0] != "♻️ 自动选择" || selectGroup.Members[1] != "DIRECT" {
		t.Errorf("节点选择组成员不对：%+v", selectGroup)
	}
	hk := byName["🇭🇰 香港节点"]
	if hk.Type != "url-test" || hk.Interval != 300 || len(hk.Members) != 1 || hk.Members[0] != "🇭🇰 香港 01" {
		t.Errorf("香港组应按正则只收香港节点：%+v", hk)
	}
	auto := byName["♻️ 自动选择"]
	if len(auto.Members) != 3 {
		t.Errorf("自动选择应包含全部 3 个节点：%+v", auto)
	}

	if len(st.Rules) != 4 {
		t.Fatalf("应有 2 条 RULE-SET + 1 条内联 + 1 条 GEOIP，实际 %d", len(st.Rules))
	}
	if st.Rules[0].Value != "LocalAreaNetwork" || st.Rules[0].Target != "🎯 全球直连" ||
		st.Rules[0].Seq != 1 || st.Rules[0].Position != byName["🎯 全球直连"].ID {
		t.Errorf("第一条规则不对：%+v", st.Rules[0])
	}
	if st.Rules[2].Type != "DOMAIN-SUFFIX" || st.Rules[2].Value != "xn--ngstr-lra8j.com" || st.Rules[2].Seq != 3 {
		t.Errorf("内联规则不对：%+v", st.Rules[2])
	}
	if st.Rules[3].Type != "GEOIP" || st.Rules[3].Value != "CN" || st.Rules[3].Seq != 4 {
		t.Errorf("GEOIP 规则不对：%+v", st.Rules[3])
	}
	if len(st.RuleSets) != 3 || st.RuleSets[1].URL != acl4ssrBase()+"Ruleset/BanAD.list" ||
		st.RuleSets[2].Name != antiadRuleSet {
		t.Errorf("规则集不对：%+v", st.RuleSets)
	}
	if st.Settings.FinalTarget != "🐟 漏网之鱼" {
		t.Errorf("兜底策略不对：%s", st.Settings.FinalTarget)
	}
}

func TestApplyUpstreamFillsMissingPolicy(t *testing.T) {
	def := upstreamDef{
		Sources: []SourceDef{{Name: "Ads", Path: "Ads.list", Policy: "🛑 广告拦截"}},
		Groups:  []GroupDef{{Name: "🎯 全球直连", Type: "select", Choices: []string{"DIRECT"}}},
		Final:   "🎯 全球直连",
	}
	st := &model.State{}
	warnings := applyUpstream(st, def)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "🛑 广告拦截") {
		t.Errorf("应提示补了缺失的策略组：%v", warnings)
	}
	if len(st.Groups) != 2 || st.Groups[1].Name != "🛑 广告拦截" {
		t.Fatalf("缺失的策略组应自动补上：%+v", st.Groups)
	}
	if st.Rules[0].Position != st.Groups[1].ID {
		t.Errorf("规则应挂到补出来的组上")
	}
}

func TestIniPresetID(t *testing.T) {
	cases := map[string]string{
		"ACL4SSR_Online_Full.ini":             PresetFull,
		"ACL4SSR_Online.ini":                  PresetDefault,
		"ACL4SSR_Online_Mini.ini":             PresetMini,
		"ACL4SSR_Online_Full_AdblockPlus.ini": "acl4ssr-online-full-adblockplus",
		"ACL4SSR_WithChinaIp_WithGFW.ini":     "acl4ssr-withchinaip-withgfw",
		"ACL4SSR.ini":                         "acl4ssr",
	}
	for file, want := range cases {
		if got := iniPresetID(file); got != want {
			t.Errorf("iniPresetID(%q)=%q，期望 %q", file, got, want)
		}
	}
	if got := iniDisplayName("ACL4SSR_Online_Mini_Ai.ini"); got != "Online Mini Ai" {
		t.Errorf("显示名不对：%q", got)
	}
}

// 上游 Clash/config 全部 ini 的 ID 必须唯一，核心三份保持固定 ID。
func TestAssignPresetIDs(t *testing.T) {
	files := []string{
		"ACL4SSR.ini", "ACL4SSR_AdblockPlus.ini", "ACL4SSR_BackCN.ini", "ACL4SSR_Mini.ini",
		"ACL4SSR_Mini_Fallback.ini", "ACL4SSR_Mini_MultiMode.ini", "ACL4SSR_Mini_NoAuto.ini",
		"ACL4SSR_NoApple.ini", "ACL4SSR_NoAuto.ini", "ACL4SSR_NoAuto_NoApple.ini",
		"ACL4SSR_NoAuto_NoApple_NoMicrosoft.ini", "ACL4SSR_NoMicrosoft.ini",
		"ACL4SSR_Online.ini", "ACL4SSR_Online_AdblockPlus.ini", "ACL4SSR_Online_Full.ini",
		"ACL4SSR_Online_Full_AdblockPlus.ini", "ACL4SSR_Online_Full_Google.ini",
		"ACL4SSR_Online_Full_MultiMode.ini", "ACL4SSR_Online_Full_Netflix.ini",
		"ACL4SSR_Online_Full_NoAuto.ini", "ACL4SSR_Online_Mini.ini",
		"ACL4SSR_Online_Mini_AdblockPlus.ini", "ACL4SSR_Online_Mini_Ai.ini",
		"ACL4SSR_Online_Mini_Fallback.ini", "ACL4SSR_Online_Mini_MultiCountry.ini",
		"ACL4SSR_Online_Mini_MultiMode.ini", "ACL4SSR_Online_Mini_NoAuto.ini",
		"ACL4SSR_Online_MultiCountry.ini", "ACL4SSR_Online_NoAuto.ini",
		"ACL4SSR_Online_NoReject.ini", "ACL4SSR_WithChinaIp.ini",
		"ACL4SSR_WithChinaIp_WithGFW.ini", "ACL4SSR_WithGFW.ini",
	}
	ids := assignPresetIDs(files)
	seen := map[string]string{}
	for file, id := range ids {
		if prev, dup := seen[id]; dup {
			t.Errorf("%s 与 %s 的 ID 撞车：%s", file, prev, id)
		}
		seen[id] = file
	}
	if ids["ACL4SSR_Online_Full.ini"] != PresetFull || ids["ACL4SSR_Online.ini"] != PresetDefault ||
		ids["ACL4SSR_Online_Mini.ini"] != PresetMini {
		t.Errorf("核心方案 ID 变了：%v", ids)
	}
	if ids["ACL4SSR_Mini.ini"] == PresetMini {
		t.Errorf("ACL4SSR_Mini.ini 不该占用核心精简方案 ID")
	}
}

// 同步来的方案自带地区组，不应再按内置地区表追加。
func TestSyncRegionsSkipsUpstreamLayout(t *testing.T) {
	st := &model.State{
		Groups:   []model.Group{{ID: "g1", Name: "🇭🇰 香港节点", Type: "url-test", Members: []string{}}},
		Subs:     []model.Subscription{{Nodes: []model.Proxy{{"name": "香港 01"}, {"name": "日本 02"}}}},
		Settings: model.Settings{PresetID: "acl4ssr-full@upstream"},
	}
	SyncRegions(st)
	if len(st.Groups) != 1 {
		t.Errorf("upstream 方案不应追加地区组，实际 %+v", st.Groups)
	}
	st.Settings.PresetID = "acl4ssr-full"
	SyncRegions(st)
	if len(st.Groups) < 2 {
		t.Errorf("内置方案应生成地区组，实际 %+v", st.Groups)
	}
}
