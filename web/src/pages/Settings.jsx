export default function SettingsPage({ state, mutate }) {
  const s = state.settings
  const set = (key, value) =>
    mutate((next) => {
      next.settings[key] = value
    })

  return (
    <>
      <div className="card">
        <div className="card-head">
          <h2>基本</h2>
          <span className="hint">改完自动保存</span>
        </div>
        <div className="grid2">
          <div className="field">
            <label>配置名</label>
            <input className="input" value={s.title} onChange={(e) => set('title', e.target.value)} />
          </div>
          <div className="field">
            <label>混合端口（mixed-port）</label>
            <input
              className="input"
              type="number"
              value={s.mixedPort}
              onChange={(e) => set('mixedPort', Number(e.target.value) || 0)}
            />
          </div>
          <div className="field">
            <label>控制端口（external-controller）</label>
            <input
              className="input"
              value={s.externalController || ''}
              placeholder="127.0.0.1:9090"
              onChange={(e) => set('externalController', e.target.value)}
            />
          </div>
          <div className="field">
            <label>控制密码（secret）</label>
            <input className="input" value={s.secret || ''} onChange={(e) => set('secret', e.target.value)} />
          </div>
          <div className="field">
            <label>最终策略（MATCH 的落点）</label>
            <select className="select" value={s.finalTarget || ''} onChange={(e) => set('finalTarget', e.target.value)}>
              <option value="">跟随最后一个组</option>
              {state.groups.map((g) => (
                <option key={g.id} value={g.name}>
                  {g.name}
                </option>
              ))}
              <option value="DIRECT">DIRECT</option>
            </select>
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
        </div>
        <div className="row" style={{ gap: 18 }}>
          <label className="row" style={{ gap: 6 }}>
            <input type="checkbox" checked={!!s.allowLan} onChange={(e) => set('allowLan', e.target.checked)} />
            允许局域网连接
          </label>
          <label className="row" style={{ gap: 6 }}>
            <input
              type="checkbox"
              checked={!s.ipv6Disabled}
              onChange={(e) => set('ipv6Disabled', !e.target.checked)}
            />
            启用 IPv6
          </label>
          <label className="row" style={{ gap: 6 }}>
            <input type="checkbox" checked={!s.disableDns} onChange={(e) => set('disableDns', !e.target.checked)} />
            加密 DNS（DoH + fake-ip）
          </label>
          <label className="row" style={{ gap: 6 }}>
            <input type="checkbox" checked={!s.tunDisabled} onChange={(e) => set('tunDisabled', !e.target.checked)} />
            启用 TUN
          </label>
          <label className="row" style={{ gap: 6 }}>
            <input
              type="checkbox"
              checked={!s.tailscaleDisabled}
              onChange={(e) => set('tailscaleDisabled', !e.target.checked)}
            />
            Tailscale 不进 TUN / 网段直连
          </label>
          <label className="row" style={{ gap: 6 }}>
            <input
              type="checkbox"
              checked={!s.localIpDisabled}
              onChange={(e) => set('localIpDisabled', !e.target.checked)}
            />
            本地 IP 直连
          </label>
          <label className="row" style={{ gap: 6 }}>
            <input
              type="checkbox"
              checked={!s.verysyncDisabled}
              onChange={(e) => set('verysyncDisabled', !e.target.checked)}
            />
            微力同步直连
          </label>
          <label className="row" style={{ gap: 6 }}>
            <input
              type="checkbox"
              checked={!s.btDisabled}
              onChange={(e) => set('btDisabled', !e.target.checked)}
            />
            常见 BT 下载直连（qBittorrent / Transmission 等）
          </label>
          <label className="row" style={{ gap: 6 }}>
            <input
              type="checkbox"
              checked={!s.commonOnlyDisabled}
              onChange={(e) => set('commonOnlyDisabled', !e.target.checked)}
            />
            仅代理常见端口
          </label>
        </div>
        <div className="note" style={{ marginTop: 8 }}>
          TUN 使用 system 栈并接管 DNS（dns-hijack any:53），需要容器有 NET_ADMIN 权限或直接跑在宿主机。
          直连类开关（Tailscale / 本地 IP / 微力同步 / BT 下载）会写在规则最前面；「仅代理常见端口」只让
          80 / 443 / 8080 / 8443 走分流，其余端口一律直连。
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
          筛选后节点数见顶栏「节点」计数。正则语法与 Go 的 regexp 一致，不区分大小写可写 <code className="mono">(?i)</code>。
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
