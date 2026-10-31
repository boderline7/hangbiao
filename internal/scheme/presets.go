// Package scheme 提供内置方案。三套方案都对齐上游 pengchujin/tower：
//
// 策略组顺序见 ConfigurationGenerator.clash —— 节点选择 / 手动切换 / 自动选择 /
// 隐藏的嵌套组 / 规则策略组（国外广告、国际流量、国内流量…）/ 地区组；
// 规则源顺序分别对应 ACL4SSR 的 ACL4SSR_Online_Full.ini、ACL4SSR_Online.ini、
// ACL4SSR_Online_Mini.ini（与上游 Resources/ACL4SSR 里那三份 ini 同一份数据）。
package scheme

import (
	"errors"
	"strings"

	"towerdocker/internal/model"
)

type Preset struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Desc   string `json:"desc"`
	Groups int    `json:"groups"`
	Rules  int    `json:"rules"`
	// Source 是 upstream 时表示这组数据来自运行时同步的 ACL4SSR ini。
	Source string `json:"source,omitempty"`
}

var ErrUnknownPreset = errors.New("未知的方案")

// 方案 ID 沿用上游 ACL4SSR_manifest.json 的键名。
const (
	PresetFull    = "acl4ssr-full"
	PresetDefault = "acl4ssr-default"
	PresetMini    = "acl4ssr-mini"
)

var Presets = []Preset{
	{ID: PresetFull, Name: "ACL4SSR 全分组", Desc: "流媒体、AI、游戏、音乐全覆盖，按节点名分地区组"},
	{ID: PresetDefault, Name: "ACL4SSR 默认", Desc: "去广告、自动测速，含国外媒体、电报、微软和苹果分流"},
	{ID: PresetMini, Name: "ACL4SSR 精简", Desc: "只保留节点选择、自动测速、直连与拦截，组数最少"},
}

func Apply(st *model.State, id string) error {
	if def, ok := lookupUpstream(id); ok {
		applyUpstream(st, def)
		st.Settings.PresetID = id + "@upstream"
		return nil
	}
	def, ok := presetDefs[id]
	if !ok {
		return ErrUnknownPreset
	}
	build(st, def.sources)
	st.Settings.PresetID = id
	return nil
}

// lookupUpstream 取运行时同步到的方案；没同步过返回 false，走内置表。
func lookupUpstream(id string) (upstreamDef, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	def, ok := syncedDefs[id]
	return def, ok
}

// 上游 ConfigurationGenerator.swift 里的嵌套组名（带 emoji 的隐藏组）。
const (
	nestedSelect = "🚀 节点选择"
	nestedManual = "🎛️ 手动切换"
	nestedAuto   = "♻️ 自动选择"
	nestedDirect = "🎯 直接连接"
)

// 上游 RulePolicy.configurationName —— 同时也是 Clash 里的策略组名。
const (
	policySelect        = "节点选择"
	policyAuto          = "自动选择"
	policyDirect        = "DIRECT"
	policyInternational = "国际流量"
	policyDomestic      = "国内流量"
	policyForeignAds    = "国外广告"
	policyAI            = "AI服务"
	policyYouTube       = "YouTube"
	policyMedia         = "国外媒体"
	policyTelegram      = "Telegram"
	policyGoogleFCM     = "Google FCM"
	policyApple         = "苹果服务"
	policyMicrosoft     = "Microsoft"
)

const ruleInterval = 86400

// antiadRuleSet 是默认预置但不引用的广告规则集（内容来自 ACL4SSR 的 BanAD.list）。
const antiadRuleSet = "antiad"

// ruleSource 一个 ACL4SSR 规则源：名字即 rule-provider 的键，path 相对 Clash/。
type ruleSource struct {
	name   string
	path   string
	policy string
}

type presetDef struct {
	sources []ruleSource
}

// acl4ssrSourcesFull 严格按 ACL4SSR_Online_Full.ini 的 ruleset 顺序排列。
var acl4ssrSourcesFull = []ruleSource{
	{"LocalAreaNetwork", "LocalAreaNetwork.list", policyDirect},
	{"UnBan", "UnBan.list", policyDirect},
	{"BanAD", "BanAD.list", policyForeignAds},
	{"BanProgramAD", "BanProgramAD.list", policyForeignAds},
	{"GoogleFCM", "Ruleset/GoogleFCM.list", policyGoogleFCM},
	{"GoogleCN", "GoogleCN.list", policyDirect},
	{"SteamCN", "Ruleset/SteamCN.list", policyDirect},
	{"Bing", "Bing.list", policyMicrosoft},
	{"OneDrive", "OneDrive.list", policyMicrosoft},
	{"Microsoft", "Microsoft.list", policyMicrosoft},
	{"Apple", "Apple.list", policyApple},
	{"Telegram", "Telegram.list", policyTelegram},
	{"AI", "Ruleset/AI.list", policyAI},
	{"OpenAi", "Ruleset/OpenAi.list", policyAI},
	{"NetEaseMusic", "Ruleset/NetEaseMusic.list", policyDomestic},
	{"Epic", "Ruleset/Epic.list", policyInternational},
	{"Origin", "Ruleset/Origin.list", policyInternational},
	{"Sony", "Ruleset/Sony.list", policyInternational},
	{"Steam", "Ruleset/Steam.list", policyInternational},
	{"Nintendo", "Ruleset/Nintendo.list", policyInternational},
	{"YouTube", "Ruleset/YouTube.list", policyYouTube},
	{"Netflix", "Ruleset/Netflix.list", policyMedia},
	{"Bahamut", "Ruleset/Bahamut.list", policyMedia},
	{"BilibiliHMT", "Ruleset/BilibiliHMT.list", policyDomestic},
	{"Bilibili", "Ruleset/Bilibili.list", policyDomestic},
	{"ChinaMedia", "ChinaMedia.list", policyDomestic},
	{"ProxyMedia", "ProxyMedia.list", policyMedia},
	{"ProxyGFWlist", "ProxyGFWlist.list", policySelect},
	{"ChinaDomain", "ChinaDomain.list", policyDirect},
	{"ChinaCompanyIp", "ChinaCompanyIp.list", policyDirect},
	{"Download", "Download.list", policyDirect},
}

// acl4ssrSourcesDefault 严格按 ACL4SSR_Online.ini 的 ruleset 顺序排列。
var acl4ssrSourcesDefault = []ruleSource{
	{"LocalAreaNetwork", "LocalAreaNetwork.list", policyDirect},
	{"UnBan", "UnBan.list", policyDirect},
	{"BanAD", "BanAD.list", policyForeignAds},
	{"BanProgramAD", "BanProgramAD.list", policyForeignAds},
	{"GoogleFCM", "Ruleset/GoogleFCM.list", policyGoogleFCM},
	{"GoogleCN", "GoogleCN.list", policyDirect},
	{"SteamCN", "Ruleset/SteamCN.list", policyDirect},
	{"Microsoft", "Microsoft.list", policyMicrosoft},
	{"Apple", "Apple.list", policyApple},
	{"Telegram", "Telegram.list", policyTelegram},
	{"ProxyMedia", "ProxyMedia.list", policyMedia},
	{"ProxyLite", "ProxyLite.list", policySelect},
	{"ChinaDomain", "ChinaDomain.list", policyDirect},
	{"ChinaCompanyIp", "ChinaCompanyIp.list", policyDirect},
}

// acl4ssrSourcesMini 严格按 ACL4SSR_Online_Mini.ini 的 ruleset 顺序排列。
// 这份 ini 里电报、国外媒体、国际代理都直接指向「节点选择」。
var acl4ssrSourcesMini = []ruleSource{
	{"LocalAreaNetwork", "LocalAreaNetwork.list", policyDirect},
	{"UnBan", "UnBan.list", policyDirect},
	{"BanAD", "BanAD.list", policyForeignAds},
	{"BanProgramAD", "BanProgramAD.list", policyForeignAds},
	{"GoogleCN", "GoogleCN.list", policyDirect},
	{"SteamCN", "Ruleset/SteamCN.list", policyDirect},
	{"Telegram", "Telegram.list", policySelect},
	{"ProxyMedia", "ProxyMedia.list", policySelect},
	{"ProxyLite", "ProxyLite.list", policySelect},
	{"ChinaDomain", "ChinaDomain.list", policyDirect},
	{"ChinaCompanyIp", "ChinaCompanyIp.list", policyDirect},
}

var presetDefs = map[string]presetDef{
	PresetFull:    {sources: acl4ssrSourcesFull},
	PresetDefault: {sources: acl4ssrSourcesDefault},
	PresetMini:    {sources: acl4ssrSourcesMini},
}

func build(st *model.State, sources []ruleSource) {
	st.Groups = nil
	st.Rules = nil
	st.RuleSets = nil
	ids := map[string]string{}

	add := func(name, typ string, members []string, hidden bool) {
		g := model.Group{
			ID:       model.NewID(),
			Name:     name,
			Type:     typ,
			Members:  members,
			Hidden:   hidden,
			Interval: regionInterval,
			URL:      regionTestURL,
		}
		if typ == "select" {
			g.Interval = 0
			g.URL = ""
		}
		st.Groups = append(st.Groups, g)
		ids[name] = g.ID
	}

	primary := []string{nestedAuto, nestedManual, "$regions", nestedDirect}
	// 1-3：可见的三个主组（顺序即上游 clash 输出顺序）
	add(policySelect, "select", primary, false)
	add("手动切换", "select", []string{"$nodes"}, false)
	add(policyAuto, "url-test", []string{"$nodes"}, false)
	// 4-7：隐藏的嵌套组，供规则策略组引用
	add(nestedSelect, "select", primary, true)
	add(nestedManual, "select", []string{"$nodes"}, true)
	add(nestedAuto, "url-test", []string{"$nodes"}, true)
	add(nestedDirect, "select", []string{"DIRECT"}, true)

	// 8+：规则策略组，顺序 = 规则源里首次出现的顺序（上游 configurablePolicies）
	// 兜底策略（国际流量）必须存在，否则 MATCH 会指向一个不存在的组。
	defaultChoices := []string{nestedSelect, nestedAuto, nestedManual, "$regions", nestedDirect}
	policies := policyOrder(sources)
	if !contains(policies, policyInternational) {
		policies = append(policies, policyInternational)
	}
	for _, policy := range policies {
		add(policy, "select", policyChoices(policy, defaultChoices), false)
	}

	// 规则：按规则源顺序，Seq 决定导出顺序
	seq := 0
	for _, src := range sources {
		seq++
		target, position := src.policy, ids[src.policy]
		if src.policy == policyDirect {
			position = ids[nestedDirect]
		}
		st.Rules = append(st.Rules, model.Rule{
			ID:       model.NewID(),
			Type:     "RULE-SET",
			Value:    src.name,
			Target:   target,
			Position: position,
			Seq:      seq,
		})
		st.RuleSets = append(st.RuleSets, model.RuleSet{
			ID:       model.NewID(),
			Name:     src.name,
			Behavior: "classical",
			URL:      acl4ssrBase() + src.path,
			Path:     "./ruleset/" + src.name + ".list",
			Interval: ruleInterval,
		})
	}
	// 广告规则集默认存在但不引用：需要时在分组里选它即可，导出前不会拉取。
	EnsureAntiAd(st)
	// GEOIP,CN 与上游一样放在所有规则源之后
	seq++
	st.Rules = append(st.Rules, model.Rule{
		ID:       model.NewID(),
		Type:     "GEOIP",
		Value:    "CN",
		Target:   policyDirect,
		Params:   "no-resolve",
		Position: ids[nestedDirect],
		Seq:      seq,
	})

	st.Settings.FinalTarget = policyInternational
	SyncRegions(st)
}

// policyOrder 按规则源顺序收集不重复的策略组；direct / 节点选择 / 自动选择
// 都复用已有的组，不再新建（否则会出现两个同名组）。
func policyOrder(sources []ruleSource) []string {
	out := make([]string, 0, 12)
	seen := map[string]bool{}
	for _, src := range sources {
		if src.policy == policyDirect || src.policy == policySelect || src.policy == policyAuto {
			continue
		}
		if seen[src.policy] {
			continue
		}
		seen[src.policy] = true
		out = append(out, src.policy)
	}
	return out
}

// policyChoices 对应上游 policyChoices(_:regionGroupNames:reject:)。
func policyChoices(policy string, defaultChoices []string) []string {
	switch policy {
	case policyForeignAds:
		return []string{"REJECT", nestedDirect, nestedSelect}
	case policyDomestic, policyApple, policyMicrosoft:
		return []string{nestedDirect, nestedSelect, nestedManual, "$regions", nestedAuto}
	}
	return defaultChoices
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func hasRuleSet(list []model.RuleSet, name string) bool {
	for _, rs := range list {
		if rs.Name == name {
			return true
		}
	}
	return false
}

// EnsureAntiAd 补上默认的 antiad 规则集（ACL4SSR 的 BanAD.list）。
// 只补缺失、不加规则引用，导出时不会拉取；旧状态启动时也靠它补齐。
func EnsureAntiAd(st *model.State) {
	if hasRuleSet(st.RuleSets, antiadRuleSet) {
		return
	}
	st.RuleSets = append(st.RuleSets, model.RuleSet{
		ID:       model.NewID(),
		Name:     antiadRuleSet,
		Behavior: "classical",
		URL:      acl4ssrBase() + "BanAD.list",
		Path:     "./ruleset/antiad.list",
		Interval: ruleInterval,
	})
}

// RuleTypes 供前端下拉使用。
var RuleTypes = []string{
	"DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "DOMAIN-REGEX",
	"IP-CIDR", "IP-CIDR6", "GEOIP", "GEOSITE", "SRC-GEOIP",
	"DST-PORT", "SRC-PORT", "PROCESS-NAME", "RULE-SET", "MATCH",
}

// Upgrade 给旧数据补新方案里才有的默认项。
func Upgrade(st *model.State) {
	if strings.TrimSpace(st.Settings.FinalTarget) == "" && len(st.Groups) > 0 {
		st.Settings.FinalTarget = st.Groups[len(st.Groups)-1].Name
	}
}
