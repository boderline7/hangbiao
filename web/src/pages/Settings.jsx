const TARGET_LABELS = {
  auto: '自动（按 UA，按 Clash 处理）',
  clash: 'Clash / mihomo',
  shadowrocket: 'Shadowrocket（小火箭）',
  passwall: 'PassWall',
}

// 每项对当前导出目标是否有效；不支持项在界面上置灰并真正禁用。
// passwall 只输出节点分享链接，除配置名 / 订阅更新间隔 / 节点筛选外全部无效。
const SUPPORT = {
  clash: {
    mixedPort: true,
    allowLan: true,
    ipv6: true,
    dns: true,
    tun: true,
    tailscale: true,
    localIp: true,
    verysync: true,
    bt: true,
    commonOnly: true,
  },
  shadowrocket: {
    mixedPort: false,
    allowLan: false,
    ipv6: true,
    dns: true,
    tun: false,
    tailscale: true,
    localIp: true,
    verysync: true,
    bt: true,
    commonOnly: true,
  },
  passwall: {},
}

function Toggle({ enabled, checked, onChange, children }) {
  return (
    <label className={'row' + (enabled ? '' : ' unsupported')} style={{ gap: 6 }}>
      <input type="checkbox" checked={checked} disabled={!enabled} onChange={onChange} />
      {children}
    </label>
  )
}

export default function SettingsPage({ state, mutate, target }) {
  const s = state.settings
  const set = (key, value) =>
    mutate((next) => {
      next.settings[key] = value
    })

  const targetLabel = TARGET_LABELS[target] || TARGET_LABELS.auto
  const support = SUPPORT[target] || SUPPORT.clash
  const ok = (key) => !!support[key]
  const targetHint = ok('tun') ? `当前导出目标：${targetLabel}` : `当前导出目标：${targetLabel}，灰色项对该客户端无效`

  return (
    <>
      <div className="card">
        <div className="card-head">
          <h2>基本</h2>
          <span className="hint">{targetHint}</span>
        </div>
        <div className="grid2">
          <div className="field">
            <label>配置名</label>
            <input className="input" value={s.title} onChange={(e) => set('title', e.target.value)} />
          </div>
          <div className={'field' + (ok('mixedPort') ? '' : ' unsupported')}>
            <label>混合端口（mixed-port）</label>
            <input
              className="input"
              type="number"
              value={s.mixedPort}
              disabled={!ok('mixedPort')}
              onChange={(e) => set('mixedPort', Number(e.target.value) || 0)}
            />
          </div>
          <div className="field">
            <label>订阅更新间隔（小时）</label>
            <input
              className="input"
              type="number"
              value={s.updateInterval}
              onChange={(e) => set('updateInterval', Number(e.target.value) || 24)}
            />
          </div>
        </div>
      </div>

      <div className="card">
        <div className="card-head">
          <h2>开关</h2>
          <span className="hint">{targetHint}</span>
        </div>
        <div className="row" style={{ gap: 18 }}>
          <Toggle enabled={ok('allowLan')} checked={!!s.allowLan} onChange={(e) => set('allowLan', e.target.checked)}>
            允许局域网连接
          </Toggle>
          <Toggle enabled={ok('ipv6')} checked={!s.ipv6Disabled} onChange={(e) => set('ipv6Disabled', !e.target.checked)}>
            启用 IPv6
          </Toggle>
          <Toggle enabled={ok('dns')} checked={!s.disableDns} onChange={(e) => set('disableDns', !e.target.checked)}>
            加密 DNS（DoH + fake-ip）
          </Toggle>
          <Toggle enabled={ok('tun')} checked={!!s.tunEnabled} onChange={(e) => set('tunEnabled', e.target.checked)}>
            启用 TUN
          </Toggle>
          <Toggle
            enabled={ok('tailscale')}
            checked={!s.tailscaleDisabled}
            onChange={(e) => set('tailscaleDisabled', !e.target.checked)}
          >
            Tailscale 不进 TUN / 网段直连
          </Toggle>
          <Toggle
            enabled={ok('localIp')}
            checked={!s.localIpDisabled}
            onChange={(e) => set('localIpDisabled', !e.target.checked)}
          >
            本地 IP 直连
          </Toggle>
          <Toggle
            enabled={ok('verysync')}
            checked={!s.verysyncDisabled}
            onChange={(e) => set('verysyncDisabled', !e.target.checked)}
          >
            微力同步直连
          </Toggle>
          <Toggle
            enabled={ok('bt')}
            checked={!s.btDisabled}
            onChange={(e) => set('btDisabled', !e.target.checked)}
          >
            常见 BT 下载直连（qBittorrent / Transmission 等）
          </Toggle>
          <Toggle
            enabled={ok('commonOnly')}
            checked={!s.commonOnlyDisabled}
            onChange={(e) => set('commonOnlyDisabled', !e.target.checked)}
          >
            仅代理常见端口
          </Toggle>
        </div>
        <div className="note" style={{ marginTop: 8 }}>
          TUN 默认关闭；开启后使用 system 栈并接管 DNS（dns-hijack any:53），需要容器有 NET_ADMIN 权限或直接跑在宿主机。
          直连类开关（Tailscale / 本地 IP / 微力同步 / BT 下载）会写在规则最前面；「仅代理常见端口」只让
          80 / 443 / 8080 / 8443 走分流，其余端口一律直连。关闭加密 DNS 后改用明文解析：国内
          223.5.5.5 / 119.29.29.29，国外 fallback 1.1.1.1 / 8.8.8.8。
        </div>
      </div>

      <div className="card">
        <div className="card-head">
          <h2>节点筛选</h2>
          <span className="hint">按节点名正则匹配，一行一条；导出、分组、订阅分发都按筛选后的节点</span>
        </div>
        <div className="grid2">
          <div className="field">
            <label>包含（为空表示全部保留）</label>
            <textarea
              className="input"
              style={{ minHeight: 80, resize: 'vertical' }}
              placeholder={'例如：香港|HK|HongKong\n例如：IEPL'}
              value={s.filterInclude || ''}
              onChange={(e) => set('filterInclude', e.target.value)}
            />
          </div>
          <div className="field">
            <label>不包含（命中任意一条即剔除）</label>
            <textarea
              className="input"
              style={{ minHeight: 80, resize: 'vertical' }}
              placeholder={'例如：剩余流量|套餐到期\n例如：官网|备用'}
              value={s.filterExclude || ''}
              onChange={(e) => set('filterExclude', e.target.value)}
            />
          </div>
        </div>
        <div className="note" style={{ marginTop: 6 }}>
          默认排除机场信息类节点：<code className="mono">客服|流量|到期|机场|剩余</code>（这一栏留空时会自动填回）。
          筛选后节点数见顶栏「节点」计数。正则语法与 Go 的 regexp 一致，不区分大小写可写{' '}
          <code className="mono">(?i)</code>。
        </div>
      </div>

      <div className="card">
        <div className="card-head">
          <h2>数据</h2>
        </div>
        <div className="note">
          全部状态保存在容器内 <code className="mono">/data/state.json</code>，备份就是复制这个文件。
          节点会随订阅刷新而被替换，策略组、规则、规则集不受影响。
        </div>
      </div>
    </>
  )
}
