// Package api 提供 REST 接口、订阅分发入口和前端静态文件。
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"towerdocker/internal/exporter"
	"towerdocker/internal/model"
	"towerdocker/internal/scheme"
	"towerdocker/internal/store"
	"towerdocker/internal/sub"
)

type Server struct {
	st     *store.Store
	static fs.FS
}

func New(st *store.Store, static fs.FS) *Server {
	return &Server{st: st, static: static}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/api/"):
		s.handleAPI(w, r)
	case p == "/sub" || strings.HasPrefix(p, "/sub/"):
		s.handleSub(w, r)
	default:
		s.handleStatic(w, r)
	}
}

// ---------- 通用 ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// stateResp 在状态外层补上筛选后的节点统计，前端顶栏与分组成员列表都用它。
type stateResp struct {
	*model.State
	NodeCount int      `json:"nodeCount"`
	NodeNames []string `json:"nodeNames"`
}

func writeState(w http.ResponseWriter, st *model.State) {
	writeJSON(w, 200, stateResp{State: st, NodeCount: len(st.Nodes()), NodeNames: st.NodeNames()})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// ---------- API ----------

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	route := strings.TrimPrefix(r.URL.Path, "/api/")
	switch {
	case route == "state":
		s.handleState(w, r)
	case route == "presets":
		if r.Method != http.MethodGet {
			fail(w, 405, "method not allowed")
			return
		}
		s.handlePresets(w)
	case route == "presets/sync":
		if r.Method != http.MethodPost {
			fail(w, 405, "method not allowed")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		res, err := scheme.Sync(ctx)
		if err != nil {
			fail(w, 502, "同步失败："+err.Error())
			return
		}
		writeJSON(w, 200, res)
	case strings.HasPrefix(route, "presets/") && strings.HasSuffix(route, "/apply"):
		s.handleApplyPreset(w, r, strings.TrimSuffix(strings.TrimPrefix(route, "presets/"), "/apply"))
	case route == "subs" && r.Method == http.MethodPost:
		s.handleAddSub(w, r)
	case route == "subs/refresh" && r.Method == http.MethodPost:
		s.refreshAllSubs()
		writeState(w, s.st.Get())
	case route == "rulesets/check" && r.Method == http.MethodPost:
		writeJSON(w, 200, s.checkRuleSets(r.Context()))
	case strings.HasPrefix(route, "subs/"):
		s.handleSubAction(w, r, strings.TrimPrefix(route, "subs/"))
	case route == "export":
		s.handleExport(w, r)
	case route == "health":
		writeJSON(w, 200, map[string]any{
			"ok":              true,
			"time":            time.Now().Format(time.RFC3339),
			"acl4ssrRevision": scheme.Revision(),
			"ruleSource":      sub.Mirror(),
		})
	default:
		fail(w, 404, "not found")
	}
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeState(w, s.st.Get())
	case http.MethodPut:
		var incoming model.State
		if err := readJSON(r, &incoming); err != nil {
			fail(w, 400, "状态解析失败："+err.Error())
			return
		}
		err := s.st.Update(func(cur *model.State) error {
			cur.Groups = incoming.Groups
			cur.Rules = incoming.Rules
			cur.RuleSets = incoming.RuleSets
			cur.Settings = incoming.Settings
			scheme.Upgrade(cur)
			return nil
		})
		if err != nil {
			fail(w, 400, "保存失败："+err.Error())
			return
		}
		writeState(w, s.st.Get())
	default:
		fail(w, 405, "method not allowed")
	}
}

func (s *Server) handlePresets(w http.ResponseWriter) {
	type presetInfo struct {
		scheme.Preset
	}
	out := make([]presetInfo, 0, len(scheme.Presets))
	for _, p := range scheme.Catalog() {
		probe := &model.State{}
		if err := scheme.Apply(probe, p.ID); err == nil {
			p.Groups = len(probe.Groups)
			p.Rules = len(probe.Rules)
		}
		p.Source = scheme.SourceOf(p.ID)
		out = append(out, presetInfo{Preset: p})
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleApplyPreset(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		fail(w, 405, "method not allowed")
		return
	}
	err := s.st.Update(func(st *model.State) error {
		return scheme.Apply(st, id)
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeState(w, s.st.Get())
}

type subRequest struct {
	URL     string `json:"url"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

func (s *Server) handleAddSub(w http.ResponseWriter, r *http.Request) {
	var req subRequest
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, "请求解析失败："+err.Error())
		return
	}
	nodes, format, traffic, trafficText, err := loadSub(req.URL, req.Content)
	if err != nil {
		fail(w, 400, "订阅读取失败："+err.Error())
		return
	}
	item := model.Subscription{
		ID:          model.NewID(),
		Name:        firstNonEmpty(req.Name, hostOf(req.URL), "订阅"),
		URL:         req.URL,
		Nodes:       nodes,
		UpdatedAt:   store.Now(),
		Format:      format,
		Traffic:     traffic,
		TrafficText: trafficText,
	}
	if err := s.st.Update(func(st *model.State) error {
		st.Subs = append(st.Subs, item)
		scheme.SyncRegions(st)
		return nil
	}); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeState(w, s.st.Get())
}

// ---------- 订阅刷新 ----------

// AutoRefresh 按设置里的「订阅更新间隔」定时刷新全部订阅；间隔为 0 时按 24 小时。
func (s *Server) AutoRefresh() {
	for {
		hours := s.st.Get().Settings.UpdateInterval
		if hours <= 0 {
			hours = 24
		}
		time.Sleep(time.Duration(hours) * time.Hour)
		total, failed := s.refreshAllSubs()
		if total > 0 {
			log.Printf("自动刷新订阅：%d 个，失败 %d 个", total, failed)
		}
	}
}

// refreshAllSubs 逐个拉取带 URL 的订阅，最后一次性写回状态。
func (s *Server) refreshAllSubs() (int, int) {
	type fetched struct {
		id                         string
		nodes                      []model.Proxy
		format, traffic, trafficTx string
		err                        error
	}
	cur := s.st.Get()
	items := make([]fetched, 0, len(cur.Subs))
	for _, sb := range cur.Subs {
		if sb.URL == "" {
			continue
		}
		nodes, format, traffic, trafficText, err := loadSub(sb.URL, "")
		items = append(items, fetched{sb.ID, nodes, format, traffic, trafficText, err})
	}
	failed := 0
	for _, it := range items {
		if it.err != nil {
			failed++
		}
	}
	now := store.Now()
	_ = s.st.Update(func(st *model.State) error {
		for _, it := range items {
			for i := range st.Subs {
				if st.Subs[i].ID != it.id {
					continue
				}
				st.Subs[i].UpdatedAt = now
				if it.err != nil {
					st.Subs[i].Error = it.err.Error()
					continue
				}
				st.Subs[i].Nodes = it.nodes
				st.Subs[i].Format = it.format
				st.Subs[i].Traffic = it.traffic
				st.Subs[i].TrafficText = it.trafficTx
				st.Subs[i].Error = ""
			}
		}
		scheme.SyncRegions(st)
		return nil
	})
	return len(items), failed
}

type ruleSetCheckItem struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Bytes  int    `json:"bytes"`
	Mirror string `json:"mirror,omitempty"`
	Error  string `json:"error,omitempty"`
}

type ruleSetCheckResp struct {
	Mirror string             `json:"mirror"`
	Total  int                `json:"total"`
	OK     int                `json:"ok"`
	Items  []ruleSetCheckItem `json:"items"`
}

// checkRuleSets 逐个拉取规则集，返回每个规则集的可用性和实际使用的加速源。
func (s *Server) checkRuleSets(ctx context.Context) ruleSetCheckResp {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	st := s.st.Get()
	res := ruleSetCheckResp{Mirror: sub.Detect(ctx), Total: len(st.RuleSets)}
	res.Items = make([]ruleSetCheckItem, 0, len(st.RuleSets))
	for _, rs := range st.RuleSets {
		body, mirror, err := sub.Fetch(ctx, rs.URL)
		item := ruleSetCheckItem{Name: rs.Name, OK: err == nil, Bytes: len(body), Mirror: mirror}
		if err != nil {
			item.Error = err.Error()
		} else {
			res.OK++
			if mirror != "" {
				res.Mirror = mirror
			}
		}
		res.Items = append(res.Items, item)
	}
	return res
}

func (s *Server) handleSubAction(w http.ResponseWriter, r *http.Request, rest string) {
	id, action, _ := strings.Cut(rest, "/")
	if id == "" {
		fail(w, 404, "not found")
		return
	}
	switch {
	case action == "" && r.Method == http.MethodDelete:
		err := s.st.Update(func(st *model.State) error {
			kept := st.Subs[:0]
			for _, sb := range st.Subs {
				if sb.ID != id {
					kept = append(kept, sb)
				}
			}
			st.Subs = kept
			scheme.SyncRegions(st)
			return nil
		})
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeState(w, s.st.Get())
	case action == "refresh" && r.Method == http.MethodPost:
		cur := s.st.Get()
		var target *model.Subscription
		for i := range cur.Subs {
			if cur.Subs[i].ID == id {
				target = &cur.Subs[i]
			}
		}
		if target == nil {
			fail(w, 404, "订阅不存在")
			return
		}
		nodes, format, traffic, trafficText, err := loadSub(target.URL, "")
		if err != nil {
			_ = s.st.Update(func(st *model.State) error {
				for i := range st.Subs {
					if st.Subs[i].ID == id {
						st.Subs[i].Error = err.Error()
						st.Subs[i].UpdatedAt = store.Now()
					}
				}
				return nil
			})
			fail(w, 502, "刷新失败："+err.Error())
			return
		}
		err = s.st.Update(func(st *model.State) error {
			for i := range st.Subs {
				if st.Subs[i].ID == id {
					st.Subs[i].Nodes = nodes
					st.Subs[i].Format = format
					st.Subs[i].Traffic = traffic
					st.Subs[i].TrafficText = trafficText
					st.Subs[i].Error = ""
					st.Subs[i].UpdatedAt = store.Now()
				}
			}
			scheme.SyncRegions(st)
			return nil
		})
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeState(w, s.st.Get())
	default:
		fail(w, 404, "not found")
	}
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	res, err := exporter.Build(s.st.Get(), format)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Type", "application/octet-stream")
		setAttachment(w, res.Filename)
		w.Write(res.Body)
		return
	}
	writeJSON(w, 200, map[string]any{
		"body":     string(res.Body),
		"filename": res.Filename,
		"warnings": res.Warnings,
	})
}

// ---------- 订阅分发 ----------

func (s *Server) handleSub(w http.ResponseWriter, r *http.Request) {
	st := s.st.Get()
	target := strings.ToLower(r.URL.Query().Get("target"))
	log.Printf("订阅请求 target=%q ua=%q", target, r.UserAgent())
	format := target
	if format == "" || format == "auto" {
		format = detectFormat(r.UserAgent())
	}
	res, err := exporter.Build(st, format)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	setAttachment(w, res.Filename)
	w.Header().Set("Profile-Update-Interval", strconv.Itoa(st.Settings.UpdateInterval))
	w.Header().Set("Cache-Control", "no-store")
	if info := trafficHeader(st); info != "" {
		w.Header().Set("Subscription-Userinfo", info)
	}
	w.Write(res.Body)
}

func detectFormat(ua string) string {
	u := strings.ToLower(ua)
	if strings.Contains(u, "passwall") {
		return "passwall"
	}
	return "clash"
}

func trafficHeader(st *model.State) string {
	for _, sb := range st.Subs {
		if sb.Traffic != "" {
			return sb.Traffic
		}
	}
	return ""
}

// setAttachment 用 ASCII 兜底文件名 + RFC 5987 的 UTF-8 文件名，避免中文在 HTTP 头里乱码。
func setAttachment(w http.ResponseWriter, filename string) {
	ascii := "hangbiao"
	for _, r := range filename {
		if r < 128 {
			ascii += string(r)
		}
	}
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", ascii, url.PathEscape(filename)))
}

// ---------- 静态文件 ----------

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	b, err := fs.ReadFile(s.static, name)
	if err != nil {
		b, err = fs.ReadFile(s.static, "index.html")
		if err != nil {
			http.Error(w, "前端未构建", http.StatusNotFound)
			return
		}
		name = "index.html"
	}
	ctype := mime.TypeByExtension(filepath.Ext(name))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	w.Header().Set("Content-Type", ctype)
	w.Write(b)
}

// ---------- 内部工具 ----------

func loadSub(rawURL, content string) ([]model.Proxy, string, string, string, error) {
	rawURL = strings.TrimSpace(rawURL)
	// 地址栏里填的是明文链接或整段订阅内容时，直接按内容解析，
	// 否则 http.NewRequest 会报 unsupported protocol scheme "hysteria2"
	if content == "" && rawURL != "" && !isHTTPURL(rawURL) {
		content, rawURL = rawURL, ""
	}
	body := []byte(content)
	traffic, trafficText := "", ""
	if len(body) == 0 {
		if rawURL == "" {
			return nil, "", "", "", fmt.Errorf("缺少订阅地址或订阅内容")
		}
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, "", "", "", err
		}
		req.Header.Set("User-Agent", "clash.meta/1.18.0 hangbiao")
		req.Header.Set("Accept", "*/*")
		client := &http.Client{Timeout: 45 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", "", "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, "", "", "", fmt.Errorf("上游返回 HTTP %d", resp.StatusCode)
		}
		body, err = io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		if err != nil {
			return nil, "", "", "", err
		}
		traffic, trafficText = humanTraffic(resp.Header.Get("Subscription-Userinfo"))
	}
	nodes, format, err := sub.Parse(body)
	if err != nil {
		return nil, "", "", "", err
	}
	return nodes, format, traffic, trafficText, nil
}

func humanTraffic(h string) (string, string) {
	if strings.TrimSpace(h) == "" {
		return "", ""
	}
	vals := map[string]float64{}
	for _, part := range strings.Split(h, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			vals[strings.ToLower(k)] = f
		}
	}
	gb := func(b float64) string { return fmt.Sprintf("%.1f GB", b/(1024*1024*1024)) }
	used := vals["upload"] + vals["download"]
	desc := fmt.Sprintf("已用 %s", gb(used))
	if vals["total"] > 0 {
		desc += fmt.Sprintf(" / 共 %s", gb(vals["total"]))
	}
	if exp := vals["expire"]; exp > 100000 {
		desc += "，到期 " + time.Unix(int64(exp), 0).Format("2006-01-02")
	}
	return h, desc
}

func isHTTPURL(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

func hostOf(rawURL string) string {
	s := rawURL
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.Index(s, ":"); i >= 0 {
		s = s[:i]
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
