// Package scheme 里的方案全部来自运行时同步的 ACL4SSR ini。
//
// 不再维护内置方案表：没有同步成功时 Catalog 返回空列表，界面也就不会出现
// 任何分组，避免把没同步下来的分组导出给客户端。
package scheme

import (
	"errors"

	"towerdocker/internal/model"
)

// Preset 一个可载入的方案，对应 ACL4SSR Clash/config 下的一份 ini。
type Preset struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Desc   string `json:"desc,omitempty"`
	Groups int    `json:"groups"`
	Rules  int    `json:"rules"`
	// Source 目前恒为 upstream，保留字段便于前端显示来源。
	Source string `json:"source,omitempty"`
	// Revision / FetchedAt 是这份方案的数据版本与同步时间。
	Revision  string `json:"revision,omitempty"`
	FetchedAt string `json:"fetchedAt,omitempty"`
}

var ErrUnknownPreset = errors.New("未知的方案（请先同步 ACL4SSR）")

// 方案 ID 沿用上游 ACL4SSR_manifest.json 的键名。
const (
	PresetFull    = "acl4ssr-full"
	PresetDefault = "acl4ssr-default"
	PresetMini    = "acl4ssr-mini"
)

// Apply 载入方案：只接受已同步到上游 ini 的方案。
func Apply(st *model.State, id string) error {
	def, ok := lookupUpstream(id)
	if !ok {
		return ErrUnknownPreset
	}
	applyUpstream(st, def)
	st.Settings.PresetID = id + "@upstream"
	return nil
}

// lookupUpstream 取运行时同步到的方案；没同步过返回 false。
func lookupUpstream(id string) (upstreamDef, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	def, ok := syncedDefs[id]
	return def, ok
}

const ruleInterval = 86400

// antiadRuleSet 是默认预置但不引用的广告规则集（内容来自 ACL4SSR 的 BanAD.list）。
const antiadRuleSet = "antiad"

// EnsureAntiAd 补上默认的 antiad 规则集（ACL4SSR 的 BanAD.list）。
// 只补缺失、不加规则引用，导出时不会拉取。
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

func hasRuleSet(list []model.RuleSet, name string) bool {
	for _, rs := range list {
		if rs.Name == name {
			return true
		}
	}
	return false
}

// RuleTypes 供前端下拉使用。
var RuleTypes = []string{
	"DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "DOMAIN-REGEX",
	"IP-CIDR", "IP-CIDR6", "GEOIP", "GEOSITE", "SRC-GEOIP",
	"DST-PORT", "SRC-PORT", "PROCESS-NAME", "RULE-SET", "MATCH",
}
