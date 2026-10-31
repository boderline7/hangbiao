package sub

import (
	"encoding/base64"
	"strings"
	"testing"

	"towerdocker/internal/model"
)

func byName(nodes []model.Proxy, name string) model.Proxy {
	for _, n := range nodes {
		if n.Name() == name {
			return n
		}
	}
	return nil
}

func TestParsePlainURIs(t *testing.T) {
	vmessJSON := `{"v":"2","ps":"vmess-测试","add":"vm.example.com","port":"443","id":"11111111-2222-3333-4444-555555555555","aid":"0","net":"ws","path":"/ws","host":"cdn.example.com","tls":"tls"}`
	body := strings.Join([]string{
		"ss://" + base64.RawURLEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:pw")) + "@ss.example.com:8388#ss-测试",
		"trojan://pw@tj.example.com:443?sni=tj.example.com&allowInsecure=1#trojan-测试",
		"vmess://" + base64.StdEncoding.EncodeToString([]byte(vmessJSON)),
		"vless://uuid@vl.example.com:443?security=reality&sni=www.example.com&pbk=pub&sid=abcd&type=ws&path=%2Fws#vless-测试",
		"hysteria2://pw@hy2.example.com:8443?sni=hy2.example.com&insecure=1&obfs=salamander&obfs-password=obfspw&pinSHA256=AA:BB#hy2-测试",
		"hy2://pw@hy2b.example.com:443#hy2-别名",
		"hysteria://hy1.example.com:36712?auth=hy1pw&peer=hy1.example.com&insecure=1&upmbps=20&downmbps=100#hy1-测试",
		"tuic://uuid:tuicpw@tuic.example.com:443?congestion_control=bbr&alpn=h3&sni=tuic.example.com&allow_insecure=1#tuic-测试",
		"anytls://anypw@anytls.example.com:8443?sni=anytls.example.com&insecure=1#anytls-测试",
	}, "\n")

	nodes, format, err := Parse([]byte(body))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if format != "uri" {
		t.Fatalf("格式应为 uri，实际 %q", format)
	}
	if len(nodes) != 9 {
		t.Fatalf("应解析出 9 个节点，实际 %d", len(nodes))
	}

	want := map[string]string{
		"ss-测试": "ss", "trojan-测试": "trojan", "vmess-测试": "vmess", "vless-测试": "vless",
		"hy2-测试": "hysteria2", "hy2-别名": "hysteria2", "hy1-测试": "hysteria",
		"tuic-测试": "tuic", "anytls-测试": "anytls",
	}
	for name, typ := range want {
		n := byName(nodes, name)
		if n == nil {
			t.Fatalf("缺少节点 %q", name)
		}
		if got := n.String("type"); got != typ {
			t.Errorf("%s 的 type 应为 %s，实际 %s", name, typ, got)
		}
	}

	hy2 := byName(nodes, "hy2-测试")
	if hy2.String("obfs") != "salamander" || hy2.String("obfs-password") != "obfspw" {
		t.Errorf("hysteria2 obfs 未解析：%v", hy2)
	}
	if hy2.Int("port") != 8443 || !hy2.Bool("skip-cert-verify") || hy2.String("sni") != "hy2.example.com" {
		t.Errorf("hysteria2 基础字段不对：%v", hy2)
	}
	if hy2.String("fingerprint") != "AA:BB" {
		t.Errorf("pinSHA256 未映射到 fingerprint：%v", hy2)
	}
	if alias := byName(nodes, "hy2-别名"); alias.Int("port") != 443 || alias.String("password") != "pw" {
		t.Errorf("hy2 别名解析不对：%v", alias)
	}

	hy1 := byName(nodes, "hy1-测试")
	if hy1.String("auth-str") != "hy1pw" || hy1.String("sni") != "hy1.example.com" || hy1.String("up") != "20" || hy1.String("down") != "100" {
		t.Errorf("hysteria v1 字段不对：%v", hy1)
	}

	tuic := byName(nodes, "tuic-测试")
	if tuic.String("uuid") != "uuid" || tuic.String("password") != "tuicpw" || tuic.String("congestion-controller") != "bbr" {
		t.Errorf("tuic 字段不对：%v", tuic)
	}

	any := byName(nodes, "anytls-测试")
	if any.String("password") != "anypw" || any.String("sni") != "anytls.example.com" || !any.Bool("skip-cert-verify") {
		t.Errorf("anytls 字段不对：%v", any)
	}
}

func TestParseBase64List(t *testing.T) {
	raw := "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:pw")) + "@a.example.com:8388#b64-ss\n" +
		"hy2://pw@b.example.com:443#b64-hy2"
	enc := base64.StdEncoding.EncodeToString([]byte(raw))
	nodes, format, err := Parse([]byte(enc))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if format != "base64" || len(nodes) != 2 {
		t.Fatalf("期望 base64 格式 2 个节点，实际 %q / %d", format, len(nodes))
	}
}

func TestParseClashPassthrough(t *testing.T) {
	body := `proxies:
  - name: yaml-hy2
    type: hysteria2
    server: h.example.com
    port: 443
    password: pw
    obfs: salamander
    obfs-password: opw
  - name: yaml-ssr
    type: ssr
    server: s.example.com
    port: 8388
    cipher: aes-256-cfb
    password: pw
    protocol: origin
    obfs: plain
`
	nodes, format, err := Parse([]byte(body))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if format != "clash" || len(nodes) != 2 {
		t.Fatalf("期望 clash 格式 2 个节点，实际 %q / %d", format, len(nodes))
	}
}

func TestParseUnknown(t *testing.T) {
	if _, _, err := Parse([]byte("这不是订阅")); err != ErrUnknownFormat {
		t.Fatalf("应返回 ErrUnknownFormat，实际 %v", err)
	}
}
