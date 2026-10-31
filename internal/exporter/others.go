package exporter

import (
	"strings"

	"towerdocker/internal/model"
)

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func wsOpts(p model.Proxy) (path, host string) {
	o, ok := p["ws-opts"].(map[string]any)
	if !ok {
		if m, ok := p["ws-opts"].(model.Proxy); ok {
			o = map[string]any(m)
			ok = true
		}
	}
	if !ok {
		return "/", ""
	}
	if v, ok := o["path"].(string); ok {
		path = v
	}
	if h, ok := o["headers"].(map[string]any); ok {
		if v, ok := h["Host"].(string); ok {
			host = v
		}
	}
	if path == "" {
		path = "/"
	}
	return path, host
}

func sniOf(p model.Proxy) string {
	if s := p.String("servername"); s != "" {
		return s
	}
	if s := p.String("sni"); s != "" {
		return s
	}
	return ""
}

func orDefaultStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
