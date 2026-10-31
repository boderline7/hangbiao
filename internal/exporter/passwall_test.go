package exporter

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"towerdocker/internal/model"
	"towerdocker/internal/sub"
)

func passwallState(props ...model.Proxy) *model.State {
	return &model.State{
		Subs:     []model.Subscription{{ID: "s1", Name: "测试", Nodes: props}},
		Settings: model.Settings{Title: "航标"},
	}
}

// 解码订阅，并确认每个链接的第一个路径段与节点的 server 一致。
func TestPassWallSubscription(t *testing.T) {
	st := passwallState(
		model.Proxy{
			"name": "🇭🇰 香港", "type": "ss", "server": "1.2.3.4", "port": 8388,
			"cipher": "aes-128-gcm", "password": "pw",
		},
		model.Proxy{
			"name": "日本 ws", "type": "vmess", "server": "5.6.7.8", "port": 443,
			"uuid": "11111111-2222-3333-4444-555555555555", "alterId": 0, "cipher": "auto",
			"tls": true, "servername": "jp.example.com", "network": "ws",
			"ws-opts": map[string]any{"path": "/ray", "headers": map[string]any{"Host": "jp.example.com"}},
		},
		model.Proxy{
			"name": "美国 reality", "type": "vless", "server": "9.9.9.9", "port": 443,
			"uuid": "aaaa", "tls": true, "servername": "us.example.com",
			"reality-opts":       map[string]any{"public-key": "pbk123", "short-id": "ab"},
			"client-fingerprint": "chrome",
		},
		model.Proxy{
			"name": "新加坡 trojan", "type": "trojan", "server": "3.3.3.3", "port": 443,
			"password": "tpw", "sni": "sg.example.com", "skip-cert-verify": true,
		},
		model.Proxy{
			"name": "hy2", "type": "hysteria2", "server": "7.7.7.7", "port": 8443,
			"password": "hpw", "sni": "hy.example.com", "skip-cert-verify": true,
			"obfs": "salamander", "obfs-password": "opw",
		},
		model.Proxy{"name": "不支持的 ssr", "type": "ssr", "server": "4.4.4.4", "port": 80},
	)

	res := PassWall(st)
	if res.Filename != "航标-passwall.txt" {
		t.Errorf("文件名不对：%s", res.Filename)
	}
	if res.Kept != 5 {
		t.Errorf("应转换 5 个节点，实际 %d", res.Kept)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "ssr") {
		t.Errorf("应有一条 ssr 跳过告警，实际 %v", res.Warnings)
	}

	text, err := base64.StdEncoding.DecodeString(string(res.Body))
	if err != nil {
		t.Fatalf("订阅不是标准 Base64：%v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(text)), "\n")
	if len(lines) != 5 {
		t.Fatalf("应有 5 行链接，实际 %d：\n%s", len(lines), text)
	}
	wantSchemes := []string{"ss://", "vmess://", "vless://", "trojan://", "hysteria2://"}
	for i, line := range lines {
		if !strings.HasPrefix(line, wantSchemes[i]) {
			t.Errorf("第 %d 行应以 %s 开头，实际 %s", i+1, wantSchemes[i], line)
		}
	}

	// ss：SIP002（userinfo 是 base64url 的 cipher:password）
	ss := strings.TrimPrefix(lines[0], "ss://")
	cred := strings.SplitN(ss, "@", 2)[0]
	if dec, err := base64.RawURLEncoding.DecodeString(cred); err != nil || string(dec) != "aes-128-gcm:pw" {
		t.Errorf("ss 凭据编码不对：%s (%v)", cred, dec)
	}
	if u, err := url.Parse(lines[0]); err != nil || u.Hostname() != "1.2.3.4" || u.Port() != "8388" {
		t.Errorf("ss 地址不对：%v", err)
	}
	if !strings.Contains(lines[0], "#%F0%9F%87%AD") {
		t.Errorf("节点名应做 URL 编码：%s", lines[0])
	}

	// vmess：解 base64 JSON，确认 ws/tls/sni 都带上了
	js, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(lines[1], "vmess://"))
	if err != nil {
		t.Fatalf("vmess 不是 Base64 JSON：%v", err)
	}
	for _, want := range []string{`"add":"5.6.7.8"`, `"net":"ws"`, `"path":"/ray"`, `"tls":"tls"`, `"sni":"jp.example.com"`} {
		if !strings.Contains(string(js), want) {
			t.Errorf("vmess JSON 缺少 %s：%s", want, js)
		}
	}

	// vless reality：security=reality 且带 pbk/sid
	for _, want := range []string{"security=reality", "pbk=pbk123", "sid=ab", "sni=us.example.com"} {
		if !strings.Contains(lines[2], want) {
			t.Errorf("vless reality 链接缺少 %s：%s", want, lines[2])
		}
	}
	// trojan / hy2 的 insecure 与 obfs 参数
	if !strings.Contains(lines[3], "allowInsecure=1") {
		t.Errorf("trojan 应带 allowInsecure：%s", lines[3])
	}
	for _, want := range []string{"insecure=1", "obfs=salamander", "obfs-password=opw", "sni=hy.example.com"} {
		if !strings.Contains(lines[4], want) {
			t.Errorf("hysteria2 链接缺少 %s：%s", want, lines[4])
		}
	}
}

// ssr:// 链接必须真的能序列化并解出来（PassWall 订阅里顺带支持）。
func TestPassWallSSRLink(t *testing.T) {
	st := passwallState(
		model.Proxy{"name": "ss", "type": "ss", "server": "1.2.3.4", "port": 8388,
			"cipher": "aes-128-gcm", "password": "pw"},
		model.Proxy{"name": "🇭🇰 ssr", "type": "ssr", "server": "4.4.4.4", "port": 8388,
			"cipher": "aes-256-cfb", "password": "pw2", "protocol": "auth_aes128_md5",
			"protocol-param": "12:abc", "obfs": "tls1.2_ticket_auth", "obfs-param": "cloud.example.com"},
	)
	res := PassWall(st)
	if res.Filename != "航标-passwall.txt" {
		t.Errorf("文件名不对：%s", res.Filename)
	}
	if res.Kept != 2 || len(res.Warnings) != 0 {
		t.Errorf("应转换 2 个节点且无告警，实际 %d / %v", res.Kept, res.Warnings)
	}
	text, err := base64.StdEncoding.DecodeString(string(res.Body))
	if err != nil {
		t.Fatalf("订阅不是标准 Base64：%v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(text)), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[1], "ssr://") {
		t.Fatalf("第二行应是 ssr 链接：%v", lines)
	}

	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(lines[1], "ssr://"))
	if err != nil {
		t.Fatalf("ssr 载荷不是 base64url：%v", err)
	}
	body, query, ok := strings.Cut(string(payload), "/?")
	if !ok {
		t.Fatalf("ssr 载荷缺少 /?：%s", payload)
	}
	fields := strings.Split(body, ":")
	if len(fields) != 6 {
		t.Fatalf("ssr 主体应有 6 段，实际 %d：%s", len(fields), body)
	}
	if fields[0] != "4.4.4.4" || fields[1] != "8388" ||
		fields[2] != "auth_aes128_md5" || fields[3] != "aes-256-cfb" ||
		fields[4] != "tls1.2_ticket_auth" {
		t.Errorf("ssr 主体字段不对：%s", body)
	}
	if dec, err := base64.RawURLEncoding.DecodeString(fields[5]); err != nil || string(dec) != "pw2" {
		t.Errorf("ssr 密码编码不对：%s (%v)", fields[5], dec)
	}
	q, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("ssr 参数解析失败：%v", err)
	}
	for _, pair := range [][2]string{
		{"remarks", "🇭🇰 ssr"}, {"obfsparam", "cloud.example.com"}, {"protoparam", "12:abc"},
	} {
		dec, err := base64.RawURLEncoding.DecodeString(q.Get(pair[0]))
		if err != nil || string(dec) != pair[1] {
			t.Errorf("ssr 参数 %s 不对：%q (%v)", pair[0], dec, err)
		}
	}
}

func TestPassWallEmpty(t *testing.T) {
	res := PassWall(passwallState())
	if res.Kept != 0 {
		t.Errorf("空订阅不应有节点，实际 %d", res.Kept)
	}
	if _, err := base64.StdEncoding.DecodeString(string(res.Body)); err != nil {
		t.Errorf("空订阅也应是合法 Base64：%v", err)
	}
}

// 导出的订阅必须能被重新解析成同样的节点，否则 PassWall 那边就是一堆无效链接。
func TestPassWallRoundTrip(t *testing.T) {
	st := passwallState(
		model.Proxy{"name": "ss", "type": "ss", "server": "1.2.3.4", "port": 8388,
			"cipher": "aes-256-gcm", "password": "pw", "udp": true},
		model.Proxy{"name": "vmess", "type": "vmess", "server": "5.6.7.8", "port": 443,
			"uuid": "11111111-2222-3333-4444-555555555555", "alterId": 0, "cipher": "auto",
			"tls": true, "servername": "jp.example.com", "network": "ws",
			"ws-opts": map[string]any{"path": "/ray", "headers": map[string]any{"Host": "jp.example.com"}}},
		model.Proxy{"name": "vless", "type": "vless", "server": "9.9.9.9", "port": 443,
			"uuid": "aaaa", "tls": true, "sni": "us.example.com", "network": "grpc",
			"grpc-opts": map[string]any{"grpc-service-name": "gun"}},
		model.Proxy{"name": "trojan", "type": "trojan", "server": "3.3.3.3", "port": 443,
			"password": "tpw", "sni": "sg.example.com"},
		model.Proxy{"name": "hy2", "type": "hysteria2", "server": "7.7.7.7", "port": 8443,
			"password": "hpw", "sni": "hy.example.com", "obfs": "salamander", "obfs-password": "opw"},
	)

	res := PassWall(st)
	nodes, format, err := sub.Parse(res.Body)
	if err != nil {
		t.Fatalf("导出内容无法被解析：%v", err)
	}
	if format != "base64" {
		t.Errorf("应识别为 base64 订阅，实际 %s", format)
	}
	if len(nodes) != 5 {
		t.Fatalf("往返后应仍是 5 个节点，实际 %d", len(nodes))
	}
	want := []struct{ name, typ, server, sni string }{
		{"ss", "ss", "1.2.3.4", ""},
		{"vmess", "vmess", "5.6.7.8", "jp.example.com"},
		{"vless", "vless", "9.9.9.9", "us.example.com"},
		{"trojan", "trojan", "3.3.3.3", "sg.example.com"},
		{"hy2", "hysteria2", "7.7.7.7", "hy.example.com"},
	}
	for i, w := range want {
		got := nodes[i]
		if got.Name() != w.name || got.String("type") != w.typ || got.String("server") != w.server {
			t.Errorf("第 %d 个节点不对：%s/%s/%s", i+1, got.Name(), got.String("type"), got.String("server"))
		}
		if w.sni != "" && sniOf(got) != w.sni {
			t.Errorf("%s 的 sni 不对：%s", w.name, sniOf(got))
		}
	}
	if n := nodes[4]; n.String("obfs") != "salamander" || n.String("obfs-password") != "opw" {
		t.Errorf("hy2 的 obfs 参数丢了：%v", n)
	}
	if n := nodes[2]; n.String("network") != "grpc" {
		t.Errorf("vless 的 grpc 传输丢了：%s", n.String("network"))
	}
}
