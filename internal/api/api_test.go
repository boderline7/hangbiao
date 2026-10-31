package api

import "testing"

func TestLoadSubAcceptsPlainLinkInURLField(t *testing.T) {
	link := "hysteria2://test-pw@hy2.example.com:8443/?sni=addons.mozilla.org&insecure=1#Hysteria2-8443"
	nodes, format, _, _, err := loadSub(link, "")
	if err != nil {
		t.Fatalf("地址栏填明文链接应能解析，实际报错：%v", err)
	}
	if format != "uri" || len(nodes) != 1 {
		t.Fatalf("期望 uri 格式 1 个节点，实际 %q / %d", format, len(nodes))
	}
	n := nodes[0]
	if n.String("type") != "hysteria2" || n.String("server") != "hy2.example.com" || n.Int("port") != 8443 {
		t.Errorf("节点基础字段不对：%v", n)
	}
	if n.String("password") != "test-pw" || n.String("sni") != "addons.mozilla.org" || !n.Bool("skip-cert-verify") {
		t.Errorf("节点鉴权字段不对：%v", n)
	}
}

func TestLoadSubRejectsEmpty(t *testing.T) {
	if _, _, _, _, err := loadSub("  ", ""); err == nil {
		t.Fatal("空地址与空内容应报错")
	}
}

func TestDetectFormat(t *testing.T) {
	cases := map[string]string{
		"PassWall/1.0":      "passwall",
		"clash.meta/1.18.0": "clash",
		"":                  "clash",
	}
	for ua, want := range cases {
		if got := detectFormat(ua); got != want {
			t.Errorf("detectFormat(%q)=%q，期望 %q", ua, got, want)
		}
	}
}

func TestIsHTTPURL(t *testing.T) {
	cases := map[string]bool{
		"https://example.com/sub": true,
		"HTTP://example.com":      true,
		"hysteria2://a@b:443":     false,
		"ss://xxx":                false,
		"clash://install-config":  false,
	}
	for in, want := range cases {
		if got := isHTTPURL(in); got != want {
			t.Errorf("isHTTPURL(%q)=%v，期望 %v", in, got, want)
		}
	}
}
