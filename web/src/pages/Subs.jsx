import { useState } from 'react'
import { api } from '../api.js'

export default function Subs({ state, replace, toast }) {
  const [url, setUrl] = useState('')
  const [name, setName] = useState('')
  const [paste, setPaste] = useState('')
  const [busy, setBusy] = useState('')
  const [open, setOpen] = useState(() => new Set())

  const add = async () => {
    if (!url.trim() && !paste.trim()) {
      toast('填订阅地址，或直接粘贴订阅内容', 'error')
      return
    }
    setBusy('add')
    try {
      const next = await api.addSub({ url: url.trim(), name: name.trim(), content: paste })
      replace(next)
      setUrl('')
      setName('')
      setPaste('')
      toast('订阅已添加')
    } catch (e) {
      toast(e.message, 'error')
    } finally {
      setBusy('')
    }
  }

  const refresh = async (id) => {
    setBusy(id)
    try {
      const next = await api.refreshSub(id)
      replace(next)
      toast('已刷新')
    } catch (e) {
      toast(e.message, 'error')
    } finally {
      setBusy('')
    }
  }

  const refreshAll = async () => {
    setBusy('all')
    try {
      const next = await api.refreshAllSubs()
      replace(next)
      const failed = next.subs.filter((s) => s.error).length
      toast(failed ? `已刷新，${failed} 个订阅失败` : '已刷新全部订阅', failed ? 'error' : 'ok')
    } catch (e) {
      toast(e.message, 'error')
    } finally {
      setBusy('')
    }
  }

  const remove = async (sub) => {
    if (!window.confirm(`删除订阅「${sub.name}」及其 ${sub.nodes.length} 个节点？`)) return
    try {
      replace(await api.deleteSub(sub.id))
      toast('已删除')
    } catch (e) {
      toast(e.message, 'error')
    }
  }

  const toggle = (id) =>
    setOpen((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  return (
    <>
      <div className="card">
        <div className="card-head">
          <h2>添加订阅</h2>
          <span className="hint">支持 Clash YAML、Base64 分享链接、明文 ss / vmess / trojan / vless / hysteria2 / hysteria / tuic / anytls</span>
        </div>
        <div className="row" style={{ marginBottom: 10 }}>
          <input
            className="input"
            style={{ flex: 1, minWidth: 220 }}
            placeholder="订阅地址，或直接粘贴节点链接"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
          <input
            className="input"
            style={{ width: 150 }}
            placeholder="备注名（可选）"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <button className="btn btn-primary" onClick={add} disabled={busy === 'add'}>
            {busy === 'add' ? '读取中…' : '添加'}
          </button>
        </div>
        <details>
          <summary className="note" style={{ cursor: 'pointer' }}>
            没有可直连的地址？展开粘贴订阅原文
          </summary>
          <textarea
            className="input"
            style={{ width: '100%', minHeight: 90, marginTop: 8, resize: 'vertical' }}
            placeholder="把订阅返回的内容整段粘贴到这里"
            value={paste}
            onChange={(e) => setPaste(e.target.value)}
          />
        </details>
      </div>

      <div className="card">
        <div className="card-head">
          <h2>已添加的订阅</h2>
          <div className="spacer" />
          <span className="hint">
            共 {state.subs.length} 个 · 每 {state.settings.updateInterval || 24} 小时自动刷新
          </span>
          <button
            className="btn btn-sm"
            onClick={refreshAll}
            disabled={busy === 'all' || state.subs.length === 0}
          >
            {busy === 'all' ? '刷新中…' : '全部刷新'}
          </button>
        </div>

        {state.subs.length === 0 && <div className="empty">还没有订阅。加一个机场订阅，节点才会出现在策略组成员里。</div>}

        {state.subs.map((sub) => (
          <div key={sub.id} style={{ borderTop: '1px solid var(--line)', padding: '12px 0' }}>
            <div className="row">
              <b>{sub.name}</b>
              <span className="badge">{sub.format || '未知格式'}</span>
              <span className="badge accent">{sub.nodes.length} 节点</span>
              {sub.trafficText && <span className="badge">{sub.trafficText}</span>}
              {sub.error ? (
                <span className="badge danger">网络失败</span>
              ) : (
                <span className="badge accent">网络正常</span>
              )}
              <div className="spacer" />
              <button className="btn btn-sm" onClick={() => toggle(sub.id)}>
                {open.has(sub.id) ? '收起节点' : '查看节点'}
              </button>
              <button className="btn btn-sm" onClick={() => refresh(sub.id)} disabled={busy === sub.id}>
                {busy === sub.id ? '刷新中…' : '刷新'}
              </button>
              <button className="btn btn-sm btn-danger" onClick={() => remove(sub)}>
                删除
              </button>
            </div>
            <div className="note" style={{ marginTop: 4 }}>
              上次更新 {sub.updatedAt || '—'}
              {sub.url ? ' · ' + sub.url : ''}
            </div>
            {sub.error && <div className="note" style={{ color: 'var(--danger)' }}>{sub.error}</div>}
            {open.has(sub.id) && (
              <div className="card" style={{ marginTop: 10, marginBottom: 0, background: 'var(--panel-2)' }}>
                {sub.nodes.length === 0 && <div className="empty">没有解析到节点</div>}
                {sub.nodes.slice(0, 400).map((n, i) => (
                  <div className="row" key={i} style={{ padding: '3px 0', fontSize: 12.5 }}>
                    <span className="badge">{n.type}</span>
                    <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{n.name}</span>
                    <span className="note mono">
                      {n.server}:{n.port}
                    </span>
                  </div>
                ))}
                {sub.nodes.length > 400 && <div className="note">仅显示前 400 个</div>}
              </div>
            )}
          </div>
        ))}
      </div>
    </>
  )
}
