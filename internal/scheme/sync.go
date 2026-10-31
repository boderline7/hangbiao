package scheme

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"towerdocker/internal/model"
)

// 运行时同步：直接从 ACL4SSR 上游拉 Clash/config 下全部 ini，解析出
// 「规则源 + 策略组」，每种 ini 都成为一个方案。这样不用重新构建镜像也能
// 换上最新规则数据；ini 里的分组类型（select / url-test …）、分组顺序、
// 正则成员筛选都原样照搬。

const (
	acl4ssrRepo     = "ACL4SSR/ACL4SSR"
	acl4ssrIniDir   = "Clash/config/"
	syncCacheFile   = "acl4ssr-sync.json"
	syncUserAgent   = "hangbiao"
	syncHTTPTimeout = 30 * time.Second
)

// 三份最常用的 ini 沿用原来的方案 ID，避免已有数据里的 presetId 对不上。
var corePresetFiles = map[string]string{
	"ACL4SSR_Online_Full.ini": PresetFull,
	"ACL4SSR_Online.ini":      PresetDefault,
	"ACL4SSR_Online_Mini.ini": PresetMini,
}

// SourceDef 一个规则源（ini 里的 ruleset 行）。内联规则（[]DOMAIN-SUFFIX…）
// 没有 Path，改用 RuleType/RuleValue 直接落到规则里。
type SourceDef struct {
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"` // 相对 Clash/，导出时替换成固定的 revision
	Policy    string `json:"policy"`         // 命中后进入的策略组
	RuleType  string `json:"ruleType,omitempty"`
	RuleValue string `json:"ruleValue,omitempty"`
}

// GroupDef 一个策略组（ini 里的 custom_proxy_group 行）。
type GroupDef struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Choices  []string `json:"choices,omitempty"` // [] 开头的固定成员
	Filters  []string `json:"filters,omitempty"` // 正则，按节点名筛选
	URL      string   `json:"url,omitempty"`
	Interval int      `json:"interval,omitempty"`
}

// upstreamDef 一份 ini 解析结果。
type upstreamDef struct {
	Name        string      `json:"name,omitempty"` // 显示名，取自 ini 文件名
	Sources     []SourceDef `json:"sources"`
	Groups      []GroupDef  `json:"groups"`
	GeoIP       string      `json:"geoip,omitempty"`
	GeoIPTarget string      `json:"geoipTarget,omitempty"`
	Final       string      `json:"final,omitempty"`
}

type syncCache struct {
	Version   int                    `json:"version"`
	Revision  string                 `json:"revision"`
	FetchedAt string                 `json:"fetchedAt"`
	Defs      map[string]upstreamDef `json:"defs"`
	Warnings  []string               `json:"warnings,omitempty"`
}

// cacheVersion 结构有变化时递增，旧缓存直接忽略并重新同步。
const cacheVersion = 2

var (
	regMu           sync.RWMutex
	syncedDefs      map[string]upstreamDef
	syncedRevision  string
	syncedFetchedAt string
	syncedWarnings  []string
	cachePath       string
)

// Revision 返回规则数据当前钉住的 ACL4SSR commit：同步过就用同步到的，
// 否则用构建期注入的。
func Revision() string {
	regMu.RLock()
	defer regMu.RUnlock()
	if syncedRevision != "" {
		return syncedRevision
	}
	return ACL4SSRRevision
}

// SourceOf 返回方案当前的数据来源：upstream（运行时同步）或 builtin（内置表）。
func SourceOf(id string) string {
	regMu.RLock()
	defer regMu.RUnlock()
	if _, ok := syncedDefs[id]; ok {
		return "upstream"
	}
	return "builtin"
}

// SetCachePath 设置同步结果的落盘位置（一般是数据目录下的 acl4ssr-sync.json）。
func SetCachePath(p string) {
	regMu.Lock()
	defer regMu.Unlock()
	cachePath = p
}

// LoadCache 启动时恢复上次同步的结果；文件不存在时什么也不做。
func LoadCache() error {
	regMu.RLock()
	p := cachePath
	regMu.RUnlock()
	if p == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var c syncCache
	if err := json.Unmarshal(b, &c); err != nil {
		return err
	}
	if c.Version < cacheVersion || len(c.Defs) == 0 {
		return nil
	}
	regMu.Lock()
	syncedDefs = c.Defs
	syncedRevision = c.Revision
	syncedFetchedAt = c.FetchedAt
	syncedWarnings = c.Warnings
	regMu.Unlock()
	return nil
}

// SyncResult 同步接口的返回。
type SyncResult struct {
	Revision  string   `json:"revision"`
	Presets   []string `json:"presets"`
	Warnings  []string `json:"warnings"`
	FetchedAt string   `json:"fetchedAt"`
	Cached    bool     `json:"cached,omitempty"`
}

// Sync 拉取 ACL4SSR 最新提交与 Clash/config 下全部 ini，每种 ini 转成一个方案。
// 同一个 revision 已同步过时直接返回缓存，避免把 GitHub 的匿名限流额度用完。
func Sync(ctx context.Context) (SyncResult, error) {
	client := &http.Client{Timeout: syncHTTPTimeout}
	rev, err := latestRevision(ctx, client)
	if err != nil {
		return SyncResult{}, err
	}

	regMu.RLock()
	if rev == syncedRevision && len(syncedDefs) > 0 {
		res := SyncResult{
			Revision:  rev,
			Presets:   presetIDs(syncedDefs),
			Warnings:  syncedWarnings,
			FetchedAt: syncedFetchedAt,
			Cached:    true,
		}
		regMu.RUnlock()
		return res, nil
	}
	regMu.RUnlock()

	files, err := listIniFiles(ctx, client, rev)
	if err != nil {
		return SyncResult{}, err
	}

	defs := map[string]upstreamDef{}
	var warnings []string
	ids := make([]string, 0, len(files))
	fileIDs := assignPresetIDs(files)
	for _, file := range files {
		text, err := fetchIni(ctx, client, rev, file)
		if err != nil {
			return SyncResult{}, fmt.Errorf("%s：%w", file, err)
		}
		def, w := parseIni(text)
		for _, item := range w {
			warnings = append(warnings, file+"："+item)
		}
		if len(def.Sources) == 0 {
			warnings = append(warnings, file+"：没有解析到 ruleset，已跳过")
			continue
		}
		def.Name = iniDisplayName(file)
		id := fileIDs[file]
		defs[id] = def
		ids = append(ids, id)
	}
	for file, id := range corePresetFiles {
		if _, ok := defs[id]; !ok {
			return SyncResult{}, fmt.Errorf("上游缺少必需方案 %s", file)
		}
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	regMu.Lock()
	syncedDefs = defs
	syncedRevision = rev
	syncedFetchedAt = now
	syncedWarnings = warnings
	p := cachePath
	regMu.Unlock()
	if err := writeCache(p, syncCache{Version: cacheVersion, Revision: rev, FetchedAt: now, Defs: defs, Warnings: warnings}); err != nil {
		warnings = append(warnings, "同步成功但写入缓存失败："+err.Error())
	}
	return SyncResult{Revision: rev, Presets: ids, Warnings: warnings, FetchedAt: now}, nil
}

// Catalog 返回内置方案 + 同步得到的上游方案，供 /api/presets 使用。
func Catalog() []Preset {
	regMu.RLock()
	defs := syncedDefs
	regMu.RUnlock()

	out := make([]Preset, 0, len(Presets)+len(defs))
	seen := make(map[string]bool, len(defs))
	for _, p := range Presets {
		seen[p.ID] = true
		out = append(out, p)
	}
	extras := make([]Preset, 0, len(defs))
	for id, def := range defs {
		if seen[id] {
			continue
		}
		extras = append(extras, Preset{
			ID:   id,
			Name: orDefault(def.Name, id),
			Desc: "同步自 ACL4SSR Clash/config",
		})
	}
	sort.Slice(extras, func(i, j int) bool { return extras[i].Name < extras[j].Name })
	return append(out, extras...)
}

func presetIDs(defs map[string]upstreamDef) []string {
	ids := make([]string, 0, len(defs))
	for id := range defs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// assignPresetIDs 给每份 ini 分配唯一方案 ID：核心三份用固定 ID，
// 其余按文件名生成，撞车时补 -2、-3。
func assignPresetIDs(files []string) map[string]string {
	used := map[string]bool{}
	for _, id := range corePresetFiles {
		used[id] = true
	}
	out := make(map[string]string, len(files))
	for _, file := range files {
		id := iniPresetID(file)
		if _, core := corePresetFiles[file]; !core {
			base := id
			for n := 2; used[id]; n++ {
				id = fmt.Sprintf("%s-%d", base, n)
			}
		}
		used[id] = true
		out[file] = id
	}
	return out
}

// iniPresetID 把 ini 文件名转成稳定的方案 ID；三份核心 ini 用固定 ID。
func iniPresetID(file string) string {
	if id, ok := corePresetFiles[file]; ok {
		return id
	}
	base := strings.TrimSuffix(file, ".ini")
	base = strings.TrimPrefix(base, "ACL4SSR_")
	if base == "ACL4SSR" {
		base = ""
	}
	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "acl4ssr"
	}
	return "acl4ssr-" + slug
}

// iniDisplayName 取去掉 ACL4SSR_ 前缀与 .ini 后缀的显示名。
func iniDisplayName(file string) string {
	base := strings.TrimSuffix(file, ".ini")
	base = strings.TrimPrefix(base, "ACL4SSR_")
	if base == "ACL4SSR" || base == "" {
		return "ACL4SSR"
	}
	return strings.ReplaceAll(base, "_", " ")
}

// listIniFiles 列出 Clash/config 下全部 ini（pref.ini 只是公用偏好片段，不算方案）。
func listIniFiles(ctx context.Context, client *http.Client, rev string) ([]string, error) {
	u := "https://api.github.com/repos/" + acl4ssrRepo + "/contents/" + acl4ssrIniDir + "?ref=" + rev
	body, err := githubGet(ctx, client, u)
	if err != nil {
		return nil, err
	}
	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.Type != "file" || !strings.HasSuffix(strings.ToLower(e.Name), ".ini") || e.Name == "pref.ini" {
			continue
		}
		files = append(files, e.Name)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("上游没有找到任何 ini")
	}
	return files, nil
}

func writeCache(p string, c syncCache) error {
	if p == "" {
		return nil
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func githubGet(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", syncUserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if token := os.Getenv("GH_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
			if sec, err := strconv.ParseInt(reset, 10, 64); err == nil {
				if wait := time.Until(time.Unix(sec, 0)); wait > 0 {
					return nil, fmt.Errorf("GitHub 限流，约 %d 分钟后可重试（配置 GITHUB_TOKEN 可提高额度）", int(wait.Minutes())+1)
				}
			}
		}
		return nil, fmt.Errorf("GitHub 拒绝访问（HTTP %d，可能触发限流，可配置 GITHUB_TOKEN）", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GitHub 返回 HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func latestRevision(ctx context.Context, client *http.Client) (string, error) {
	u := "https://api.github.com/repos/" + acl4ssrRepo + "/commits?per_page=1"
	body, err := githubGet(ctx, client, u)
	if err != nil {
		return "", err
	}
	var commits []struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(body, &commits); err != nil {
		return "", err
	}
	if len(commits) == 0 || commits[0].SHA == "" {
		return "", fmt.Errorf("GitHub 没有返回提交")
	}
	return commits[0].SHA, nil
}

func fetchIni(ctx context.Context, client *http.Client, rev, name string) (string, error) {
	u := "https://api.github.com/repos/" + acl4ssrRepo + "/contents/" + acl4ssrIniDir + name + "?ref=" + rev
	body, err := githubGet(ctx, client, u)
	if err != nil {
		return "", err
	}
	var f struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &f); err != nil {
		return "", err
	}
	// GitHub 的 content 是带换行的标准 Base64
	raw := strings.ReplaceAll(f.Content, "\n", "")
	dec, err := decodeBase64(raw)
	if err != nil {
		return "", err
	}
	return string(dec), nil
}

// ---------- ini 解析 ----------

func parseIni(text string) (upstreamDef, []string) {
	var def upstreamDef
	var warnings []string
	seen := map[string]bool{}
	lower := map[string]bool{}

	for _, line := range strings.Split(text, "\n") {
		s := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if s == "" || strings.HasPrefix(s, ";") || strings.HasPrefix(s, "#") {
			continue
		}
		key, val, ok := strings.Cut(s, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "ruleset":
			target, raw, ok := strings.Cut(val, ",")
			if !ok {
				warnings = append(warnings, "无法解析的 ruleset 行："+s)
				continue
			}
			target = strings.TrimSpace(target)
			raw = strings.TrimSpace(raw)
			if strings.HasPrefix(raw, "[]") {
				// []GEOIP,CN / []FINAL / []DOMAIN-SUFFIX,xxx 这类内置规则
				inner := strings.TrimSpace(strings.TrimPrefix(raw, "[]"))
				upper := strings.ToUpper(inner)
				switch {
				case strings.HasPrefix(upper, "GEOIP"):
					parts := strings.Split(inner, ",")
					if len(parts) >= 2 {
						def.GeoIP = strings.TrimSpace(parts[1])
						def.GeoIPTarget = target
					} else {
						warnings = append(warnings, "GEOIP 规则缺少地区："+s)
					}
				case strings.HasPrefix(upper, "FINAL"):
					def.Final = target
				default:
					ruleType, ruleValue, ok := strings.Cut(inner, ",")
					if !ok || strings.TrimSpace(ruleValue) == "" {
						warnings = append(warnings, "跳过不支持的规则："+inner)
						continue
					}
					def.Sources = append(def.Sources, SourceDef{
						RuleType:  strings.TrimSpace(ruleType),
						RuleValue: strings.TrimSpace(ruleValue),
						Policy:    target,
					})
				}
				continue
			}
			name, path, err := splitRulesetURL(raw)
			if err != nil {
				warnings = append(warnings, err.Error())
				continue
			}
			if seen[name] || lower[strings.ToLower(name)] {
				warnings = append(warnings, "规则源重名，已跳过："+raw)
				continue
			}
			seen[name] = true
			lower[strings.ToLower(name)] = true
			def.Sources = append(def.Sources, SourceDef{Name: name, Path: path, Policy: target})

		case "custom_proxy_group":
			gd, w := parseGroupDef(val)
			if gd.Name == "" {
				if w != "" {
					warnings = append(warnings, w)
				}
				continue
			}
			def.Groups = append(def.Groups, gd)
			if w != "" {
				warnings = append(warnings, w)
			}
		}
	}
	return def, warnings
}

func splitRulesetURL(raw string) (name, path string, err error) {
	const marker = "/Clash/"
	i := strings.Index(raw, marker)
	if i < 0 {
		return "", "", fmt.Errorf("无法识别规则源路径：%s", raw)
	}
	path = strings.TrimSpace(raw[i+len(marker):])
	if path == "" {
		return "", "", fmt.Errorf("规则源路径为空：%s", raw)
	}
	base := path
	if j := strings.LastIndex(base, "/"); j >= 0 {
		base = base[j+1:]
	}
	for _, ext := range []string{".list", ".yaml", ".yml", ".txt"} {
		base = strings.TrimSuffix(base, ext)
	}
	if base == "" {
		return "", "", fmt.Errorf("规则源文件名为空：%s", raw)
	}
	return base, path, nil
}

func parseGroupDef(val string) (GroupDef, string) {
	parts := strings.Split(val, "`")
	if len(parts) < 2 {
		return GroupDef{}, "无法解析的分组行：" + val
	}
	gd := GroupDef{
		Name: strings.TrimSpace(parts[0]),
		Type: normalizeGroupType(strings.TrimSpace(parts[1])),
	}
	gotFilter := false
	for _, raw := range parts[2:] {
		f := strings.TrimSpace(raw)
		if f == "" {
			continue
		}
		if strings.HasPrefix(f, "[]") {
			if m := strings.TrimSpace(strings.TrimPrefix(f, "[]")); m != "" {
				gd.Choices = append(gd.Choices, m)
			}
			continue
		}
		if !gotFilter {
			gotFilter = true
			gd.Filters = append(gd.Filters, f)
			continue
		}
		// 后面的字段是测速地址与间隔，只对非 select 组有意义
		if gd.Type == "select" {
			gd.Filters = append(gd.Filters, f)
			continue
		}
		switch {
		case strings.HasPrefix(strings.ToLower(f), "http"):
			gd.URL = f
		case leadingInt(f) > 0:
			gd.Interval = leadingInt(f)
		}
	}
	if gd.Type != "select" {
		if gd.Interval == 0 {
			gd.Interval = regionInterval
		}
		if gd.URL == "" {
			gd.URL = regionTestURL
		}
	}
	return gd, ""
}

func normalizeGroupType(t string) string {
	switch strings.ToLower(t) {
	case "select", "url-test", "fallback", "load-balance":
		return strings.ToLower(t)
	default:
		// relay / 未知类型都按手动选择处理，至少不会让客户端报错
		return "select"
	}
}

func leadingInt(s string) int {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0
	}
	n, _ := strconv.Atoi(s[:i])
	return n
}

// ---------- 按 ini 构建方案 ----------

// ApplyUpstream 用同步到的定义重建分组、规则与规则集。
func applyUpstream(st *model.State, def upstreamDef) []string {
	var warnings []string
	// 复制一份分组切片，避免补组时改到缓存里的定义
	groups := make([]GroupDef, len(def.Groups))
	copy(groups, def.Groups)
	def.Groups = groups
	ids := map[string]string{}
	present := map[string]bool{}
	for _, gd := range def.Groups {
		present[gd.Name] = true
	}

	// 规则指向的策略组如果在 ini 里没定义，就补一个选择组，避免导出的配置报
	// proxy [xxx] not found。
	for _, src := range def.Sources {
		if src.Policy == "" || src.Policy == "DIRECT" || src.Policy == "REJECT" {
			continue
		}
		if present[src.Policy] {
			continue
		}
		present[src.Policy] = true
		def.Groups = append(def.Groups, GroupDef{
			Name:    src.Policy,
			Type:    "select",
			Choices: []string{"$nodes", "DIRECT"},
		})
		warnings = append(warnings, fmt.Sprintf("规则指向的组「%s」在 ini 里没有定义，已补一个选择组", src.Policy))
	}
	if def.Final != "" && !present[def.Final] {
		warnings = append(warnings, fmt.Sprintf("兜底组「%s」在 ini 里没有定义", def.Final))
	}
	if def.GeoIPTarget != "" && !present[def.GeoIPTarget] {
		warnings = append(warnings, fmt.Sprintf("GEOIP 规则的组「%s」在 ini 里没有定义", def.GeoIPTarget))
	}

	st.Groups = nil
	st.Rules = nil
	st.RuleSets = nil
	nodes := st.Nodes()

	for _, gd := range def.Groups {
		g := model.Group{ID: model.NewID(), Name: gd.Name, Type: gd.Type}
		if g.Type == "" {
			g.Type = "select"
		}
		if g.Type != "select" {
			g.URL = orDefault(gd.URL, regionTestURL)
			g.Interval = orDefaultInt(gd.Interval, regionInterval)
		}
		var members []string
		seen := map[string]bool{}
		appendMember := func(name string) {
			if name == "" || seen[name] {
				return
			}
			seen[name] = true
			members = append(members, name)
		}
		for _, c := range gd.Choices {
			appendMember(c)
		}
		for _, f := range gd.Filters {
			matched := filterNodeNames(f, nodes)
			if len(matched) == 0 {
				if f != ".*" {
					warnings = append(warnings, fmt.Sprintf("「%s」的正则 %s 没有匹配到节点", gd.Name, f))
				}
				continue
			}
			for _, n := range matched {
				appendMember(n)
			}
		}
		if len(members) == 0 {
			if g.Type == "select" {
				members = []string{"DIRECT"}
			} else {
				members = []string{"$nodes"}
				warnings = append(warnings, fmt.Sprintf("「%s」没有可用节点，暂用全部节点", gd.Name))
			}
		}
		g.Members = members
		st.Groups = append(st.Groups, g)
		ids[g.Name] = g.ID
	}

	// 组引用必须都存在，否则 mihomo 直接报 proxy not found。
	// 成员既可以是节点名（正则匹配来的），也可以是别的组名。
	nodeSet := map[string]bool{}
	for _, p := range nodes {
		nodeSet[p.Name()] = true
	}
	for i := range st.Groups {
		g := &st.Groups[i]
		kept := make([]string, 0, len(g.Members))
		for _, m := range g.Members {
			switch {
			case m == "$nodes" || m == "DIRECT" || m == "REJECT" || nodeSet[m]:
				kept = append(kept, m)
			default:
				if _, ok := ids[m]; ok {
					kept = append(kept, m)
				} else {
					warnings = append(warnings, fmt.Sprintf("「%s」引用了不存在的组「%s」，已移除", g.Name, m))
				}
			}
		}
		if len(kept) == 0 {
			kept = []string{"DIRECT"}
		}
		g.Members = kept
	}

	seq := 0
	for _, src := range def.Sources {
		seq++
		if src.RuleType != "" {
			st.Rules = append(st.Rules, model.Rule{
				ID:       model.NewID(),
				Type:     src.RuleType,
				Value:    src.RuleValue,
				Target:   src.Policy,
				Position: ids[src.Policy],
				Seq:      seq,
			})
			continue
		}
		rule := model.Rule{
			ID:       model.NewID(),
			Type:     "RULE-SET",
			Value:    src.Name,
			Target:   src.Policy,
			Position: ids[src.Policy],
			Seq:      seq,
		}
		st.Rules = append(st.Rules, rule)
		st.RuleSets = append(st.RuleSets, model.RuleSet{
			ID:       model.NewID(),
			Name:     src.Name,
			Behavior: "classical",
			URL:      acl4ssrBase() + src.Path,
			Path:     "./ruleset/" + src.Name + ".list",
			Interval: ruleInterval,
		})
	}
	// 与内置预设一致：默认带上广告规则集但不引用。
	EnsureAntiAd(st)
	if def.GeoIP != "" {
		seq++
		target := orDefault(def.GeoIPTarget, "DIRECT")
		st.Rules = append(st.Rules, model.Rule{
			ID:       model.NewID(),
			Type:     "GEOIP",
			Value:    def.GeoIP,
			Target:   target,
			Params:   "no-resolve",
			Position: ids[target],
			Seq:      seq,
		})
	}
	// 兜底：ini 的 FINAL 组 > 最后一个组
	final := def.Final
	if final == "" || ids[final] == "" {
		if len(st.Groups) > 0 {
			final = st.Groups[len(st.Groups)-1].Name
		} else {
			final = "DIRECT"
		}
	}
	st.Settings.FinalTarget = final
	return warnings
}

func filterNodeNames(pattern string, nodes []model.Proxy) []string {
	if pattern == ".*" || pattern == "" {
		out := make([]string, 0, len(nodes))
		for _, p := range nodes {
			out = append(out, p.Name())
		}
		return out
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		// 不是合法正则时按关键字包含处理，尽量减少整组丢失
		out := make([]string, 0, len(nodes))
		for _, p := range nodes {
			if strings.Contains(p.Name(), pattern) {
				out = append(out, p.Name())
			}
		}
		return out
	}
	out := make([]string, 0, len(nodes))
	for _, p := range nodes {
		if re.MatchString(p.Name()) {
			out = append(out, p.Name())
		}
	}
	return out
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

func orDefaultInt(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// UsesBuiltinRegions 判断当前方案是否还需要按内置地区表重建地区组。
// 同步来的 ini 自带地区组（🇭🇰 香港节点 之类），不再额外生成。
func UsesBuiltinRegions(st *model.State) bool {
	return !strings.Contains(st.Settings.PresetID, "@upstream")
}
