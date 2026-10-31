// Package model 定义航标（hangbiao）的核心数据结构。
//
// 关键设计：策略组是一个有序列表，下标即匹配优先级。规则不维护自己的
// 全局顺序，而是挂在某个策略组的位置上（Rule.Position = 组 ID）。导出时
// 按组的先后顺序把各组的规则依次摊平成 rules 数组。要让一条规则排在
// 「全球直连」前面，只需要把承载它的那个组拖到全球直连上面。
package model

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"sync"
)

func NewID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}

// Proxy 是原样透传的节点字典（来自订阅），只在需要读名字时用方法取。
type Proxy map[string]any

func (p Proxy) Name() string {
	if v, ok := p["name"].(string); ok {
		return v
	}
	return ""
}

func (p Proxy) String(key string) string {
	if v, ok := p[key].(string); ok {
		return v
	}
	return ""
}

func (p Proxy) Int(key string) int {
	switch v := p[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

func (p Proxy) Bool(key string) bool {
	if v, ok := p[key].(bool); ok {
		return v
	}
	return false
}

// Group 策略组。顺序由所在切片下标决定。
type Group struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"` // select | url-test | fallback | load-balance
	Icon     string   `json:"icon,omitempty"`
	URL      string   `json:"url,omitempty"`
	Interval int      `json:"interval,omitempty"`
	Members  []string `json:"members"`
	Note     string   `json:"note,omitempty"`
	// Hidden 对应 mihomo 的 hidden: true，用于地区组的「延迟优选」等内部组。
	Hidden bool `json:"hidden,omitempty"`
}

// Rule 一条规则。Position 决定它在导出结果中的位置（哪个组的段里），
// Target 是命中后走向哪里（组名 / REJECT / DIRECT）。
// Seq 非 0 时按 Seq 排序，用于上游那种「规则按规则源顺序排列」的方案。
type Rule struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	Target   string `json:"target"`
	Position string `json:"position"`
	Params   string `json:"params,omitempty"`
	Seq      int    `json:"seq,omitempty"`
}

func (r Rule) Line() string {
	parts := []string{r.Type, r.Value, r.Target}
	if r.Params != "" {
		parts = append(parts, r.Params)
	}
	return strings.Join(parts, ",")
}

// RuleSet 对应客户端的 rule-providers。
type RuleSet struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Behavior string `json:"behavior"` // domain | ipcidr | classical
	Format   string `json:"format"`   // yaml | text | mrs
	URL      string `json:"url"`
	Path     string `json:"path"`
	Interval int    `json:"interval"`
}

type Subscription struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	Nodes       []Proxy `json:"nodes"`
	UpdatedAt   string  `json:"updatedAt"`
	Error       string  `json:"error,omitempty"`
	Traffic     string  `json:"traffic,omitempty"`
	TrafficText string  `json:"trafficText,omitempty"`
	Format      string  `json:"format,omitempty"`
}

type Settings struct {
	Title              string `json:"title"`
	MixedPort          int    `json:"mixedPort"`
	Mode               string `json:"mode"`
	LogLevel           string `json:"logLevel"`
	AllowLAN           bool   `json:"allowLan"`
	DisableDNS         bool   `json:"disableDns,omitempty"`
	TunDisabled        bool   `json:"tunDisabled,omitempty"`
	TailscaleDisabled  bool   `json:"tailscaleDisabled,omitempty"`
	LocalIPDisabled    bool   `json:"localIpDisabled,omitempty"`
	VerysyncDisabled   bool   `json:"verysyncDisabled,omitempty"`
	BtDisabled         bool   `json:"btDisabled,omitempty"`
	CommonOnlyDisabled bool   `json:"commonOnlyDisabled,omitempty"`
	UpdateInterval     int    `json:"updateInterval"`
	IPv6Disabled       bool   `json:"ipv6Disabled,omitempty"`
	// FilterInclude / FilterExclude 是节点名正则，一行一条；包含为空表示全收，
	// 命中去重后的节点名即生效（导出、分组、订阅分发都走这份筛选结果）。
	FilterInclude string `json:"filterInclude,omitempty"`
	FilterExclude string `json:"filterExclude,omitempty"`
	// PresetID 记录当前方案来源，例如 acl4ssr-full 或 acl4ssr-full@upstream
	// （后者表示分组/规则来自运行时同步的 ACL4SSR ini）。
	PresetID string `json:"presetId,omitempty"`
}

type State struct {
	Version  int            `json:"version"`
	Subs     []Subscription `json:"subs"`
	Groups   []Group        `json:"groups"`
	Rules    []Rule         `json:"rules"`
	RuleSets []RuleSet      `json:"ruleSets"`
	Settings Settings       `json:"settings"`
}

// Nodes 合并所有订阅的节点，按名字去重（后者加序号后缀），再按全局筛选规则过滤。
func (s *State) Nodes() []Proxy {
	seen := map[string]int{}
	out := make([]Proxy, 0, 64)
	for _, sub := range s.Subs {
		for _, p := range sub.Nodes {
			name := p.Name()
			if name == "" {
				continue
			}
			if !s.nodeAllowed(name) {
				continue
			}
			if n, ok := seen[name]; ok {
				seen[name] = n + 1
				clone := Proxy{}
				for k, v := range p {
					clone[k] = v
				}
				clone["name"] = name + " " + itoa(n+1)
				out = append(out, clone)
				continue
			}
			seen[name] = 1
			out = append(out, p)
		}
	}
	return out
}

// filterCache 缓存已编译的正则，避免每次导出都重新 Compile。
var filterCache sync.Map

func compilePatterns(raw string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if v, ok := filterCache.Load(line); ok {
			out = append(out, v.(*regexp.Regexp))
			continue
		}
		re, err := regexp.Compile(line)
		if err != nil {
			continue
		}
		filterCache.Store(line, re)
		out = append(out, re)
	}
	return out
}

// nodeAllowed：包含列表非空时必须命中其一，随后任一排除规则命中即丢弃。
func (s *State) nodeAllowed(name string) bool {
	if inc := compilePatterns(s.Settings.FilterInclude); len(inc) > 0 {
		matched := false
		for _, re := range inc {
			if re.MatchString(name) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, re := range compilePatterns(s.Settings.FilterExclude) {
		if re.MatchString(name) {
			return false
		}
	}
	return true
}

// NodeNames 返回去重后的节点名清单。
func (s *State) NodeNames() []string {
	ns := s.Nodes()
	out := make([]string, 0, len(ns))
	for _, p := range ns {
		out = append(out, p.Name())
	}
	return out
}

func (s *State) GroupIndex() map[string]int {
	idx := make(map[string]int, len(s.Groups))
	for i, g := range s.Groups {
		idx[g.ID] = i
	}
	return idx
}

// RulesByGroup 把规则按 Position 分桶，桶内保持原有相对顺序。
func (s *State) RulesByGroup() map[string][]Rule {
	out := make(map[string][]Rule, len(s.Groups))
	for _, r := range s.Rules {
		out[r.Position] = append(out[r.Position], r)
	}
	return out
}

// OrderedRules 摊平所有规则。规则带 Seq 时按 Seq 排（上游方案按规则源顺序），
// Seq 为 0 的规则（旧数据或手工新增）排在带 Seq 的后面，仍按组顺序。
func (s *State) OrderedRules() []Rule {
	numbered := false
	for _, r := range s.Rules {
		if r.Seq > 0 {
			numbered = true
			break
		}
	}
	fallback := s.rulesByGroupOrder()
	if !numbered {
		return fallback
	}
	head, tail := make([]Rule, 0, len(s.Rules)), make([]Rule, 0, len(s.Rules))
	for _, r := range fallback {
		if r.Seq > 0 {
			head = append(head, r)
		} else {
			tail = append(tail, r)
		}
	}
	sort.SliceStable(head, func(i, j int) bool { return head[i].Seq < head[j].Seq })
	return append(head, tail...)
}

func (s *State) rulesByGroupOrder() []Rule {
	buckets := s.RulesByGroup()
	out := make([]Rule, 0, len(s.Rules))
	for _, g := range s.Groups {
		out = append(out, buckets[g.ID]...)
		delete(buckets, g.ID)
	}
	orphan := make([]Rule, 0)
	for _, rs := range buckets {
		orphan = append(orphan, rs...)
	}
	return append(out, orphan...)
}

// DefaultFilterExclude 是默认排除的机场信息类节点名（客服 / 流量 / 到期 / 机场 / 剩余）。
const DefaultFilterExclude = `客服|流量|到期|机场|剩余`

// FinalTarget 返回 MATCH 的落点：始终是最后一个策略组，不再单独维护一个设置项。
func (s *State) FinalTarget() string {
	if len(s.Groups) > 0 {
		return s.Groups[len(s.Groups)-1].Name
	}
	return "DIRECT"
}

func (s *State) GroupByName(name string) *Group {
	for i := range s.Groups {
		if s.Groups[i].Name == name {
			return &s.Groups[i]
		}
	}
	return nil
}

func (s *State) RuleSetByID(id string) *RuleSet {
	for i := range s.RuleSets {
		if s.RuleSets[i].ID == id {
			return &s.RuleSets[i]
		}
	}
	return nil
}

// UsedRuleSets 返回被规则实际引用的规则集名字。
func (s *State) UsedRuleSets() map[string]bool {
	used := map[string]bool{}
	for _, r := range s.Rules {
		if strings.EqualFold(r.Type, "RULE-SET") {
			used[r.Value] = true
		}
	}
	return used
}

// Normalize 补齐缺失的 ID 与默认值，保证任何一次保存后状态自洽。
func (s *State) Normalize() {
	if s.Version == 0 {
		s.Version = 1
	}
	// 空切片不能留 nil：JSON 会序列化成 null，前端按数组遍历会直接报错
	if s.Subs == nil {
		s.Subs = []Subscription{}
	}
	if s.Groups == nil {
		s.Groups = []Group{}
	}
	if s.Rules == nil {
		s.Rules = []Rule{}
	}
	if s.RuleSets == nil {
		s.RuleSets = []RuleSet{}
	}
	for i := range s.Groups {
		if s.Groups[i].ID == "" {
			s.Groups[i].ID = NewID()
		}
		if s.Groups[i].Type == "" {
			s.Groups[i].Type = "select"
		}
		if s.Groups[i].Members == nil {
			s.Groups[i].Members = []string{}
		}
	}
	for i := range s.Rules {
		if s.Rules[i].ID == "" {
			s.Rules[i].ID = NewID()
		}
		if s.Rules[i].Params == "" && needsNoResolve(s.Rules[i].Type) {
			s.Rules[i].Params = "no-resolve"
		}
	}
	for i := range s.RuleSets {
		if s.RuleSets[i].ID == "" {
			s.RuleSets[i].ID = NewID()
		}
		if s.RuleSets[i].Behavior == "" {
			s.RuleSets[i].Behavior = "domain"
		}
		// ACL4SSR 的 .list 都是纯文本规则行，按 yaml 解析会报 payload 缺失。
		if s.RuleSets[i].Format == "yaml" && strings.HasSuffix(s.RuleSets[i].URL, ".list") {
			s.RuleSets[i].Format = "text"
		}
		if s.RuleSets[i].Format == "" {
			s.RuleSets[i].Format = "text"
		}
		if s.RuleSets[i].Interval == 0 {
			s.RuleSets[i].Interval = 86400
		}
		if s.RuleSets[i].Path == "" {
			s.RuleSets[i].Path = "./ruleset/" + s.RuleSets[i].Name + "." + s.RuleSets[i].Format
		}
	}
	if s.Settings.MixedPort == 0 {
		s.Settings.MixedPort = 7890
	}
	if s.Settings.Mode == "" {
		s.Settings.Mode = "rule"
	}
	if s.Settings.LogLevel == "" {
		s.Settings.LogLevel = "info"
	}
	if s.Settings.UpdateInterval == 0 {
		s.Settings.UpdateInterval = 24
	}
	if s.Settings.Title == "" || s.Settings.Title == "塔台-docker" {
		s.Settings.Title = "航标"
	}
	// 机场信息类节点（客服、流量、到期、机场、剩余）默认不参与导出。
	if strings.TrimSpace(s.Settings.FilterExclude) == "" {
		s.Settings.FilterExclude = DefaultFilterExclude
	}
}

func needsNoResolve(t string) bool {
	switch strings.ToUpper(t) {
	case "IP-CIDR", "IP-CIDR6", "IP6-CIDR", "GEOIP":
		return true
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 4)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		return "-" + string(buf)
	}
	return string(buf)
}

// SortRuleSetsByName 供界面稳定展示。
func (s *State) SortRuleSetsByName() {
	sort.SliceStable(s.RuleSets, func(i, j int) bool {
		return s.RuleSets[i].Name < s.RuleSets[j].Name
	})
}
