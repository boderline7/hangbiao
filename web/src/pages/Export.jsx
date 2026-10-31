import { useEffect, useRef, useState } from 'react'
import { api } from '../api.js'

const FORMATS = [
  ['auto', '自动（按 UA）'],
  ['clash', 'Clash / mihomo'],
  ['passwall', 'PassWall'],
]

const SHARE_FORMATS = new Set(['passwall'])

async function copyText(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // 走下面的兜底
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}

export default function ExportPage({ state }) {
  const [format, setFormat] = useState('auto')
  const [data, setData] = useState(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [base, setBase] = useState(() => localStorage.getItem('td-base') || window.location.origin)
  const [copied, setCopied] = useState('')
  const urlRef = useRef(null)

  useEffect(() => {
    let alive = true
    setLoading(true)
    setError('')
    api
      .exportConfig(format)
      .then((res) => {
        if (alive) setData(res)
      })
      .catch((e) => {
        if (alive) setError(e.message)
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [format, state.groups, state.rules, state.ruleSets, state.settings])

  // 订阅地址跟着当前客户端格式走；默认 auto，由服务器按 UA 判断。
  const subURL = `${base.replace(/\/$/, '')}/sub?target=${format}`

  const copy = async (text, tag) => {
    const ok = await copyText(text)
    setCopied(ok ? tag : 'fail')
    window.setTimeout(() => setCopied(''), 2600)
    return ok
  }

  // Base64 链接订阅直接看原文没意义，预览里给解码后的链接列表。
  let preview = data ? data.body : ''
  if (data && SHARE_FORMATS.has(format)) {
    try {
      preview = atob(data.body.replace(/\s/g, ''))
    } catch {
      preview = data.body
    }
  }

  return (
    <>
      <div className="card">
        <div className="card-head">
          <h2>订阅地址</h2>
          <span className="hint">地址会跟着下面切换的客户端改后缀；默认 auto，由服务器按 UA 判断格式</span>
        </div>
        <div className="row">
          <input
            className="input"
            style={{ flex: 1, minWidth: 220 }}
            value={base}
            onChange={(e) => {
              setBase(e.target.value)
              localStorage.setItem('td-base', e.target.value)
            }}
            title="如果客户端走的是别的域名或外网地址，在这里改"
          />
          <input
            ref={urlRef}
            className="input link"
            style={{ flexBasis: '100%' }}
            value={subURL}
            readOnly
            onClick={(e) => e.currentTarget.select()}
            title="点击即全选"
          />
          <button
            className="btn"
            onClick={async () => {
              const ok = await copy(subURL, 'sub')
              if (!ok && urlRef.current) {
                urlRef.current.focus()
                urlRef.current.select()
              }
            }}
          >
            {copied === 'sub' ? '已复制' : copied === 'fail' ? '复制失败，地址已全选' : '复制订阅地址'}
          </button>
          {copied === 'fail' && <span className="note">浏览器不允许自动复制，按 Ctrl+C / ⌘C 即可</span>}
          <span className="note">
            可选后缀：auto / clash / passwall，例如 ?target=passwall
          </span>
        </div>
      </div>

      <div className="card">
        <div className="card-head">
          <h2>导出配置</h2>
          <div className="spacer" />
          <div className="seg">
            {FORMATS.map(([id, label]) => (
              <button key={id} className={format === id ? 'active' : ''} onClick={() => setFormat(id)}>
                {label}
              </button>
            ))}
          </div>
          <a className="btn" href={api.downloadUrl(format)}>
            下载文件
          </a>
        </div>

        {loading && <div className="empty">生成中…</div>}
        {error && <div className="empty" style={{ color: 'var(--danger)' }}>{error}</div>}

        {!loading && data && (
          <>
            {data.warnings && data.warnings.length > 0 && (
              <div className="card" style={{ background: 'var(--panel-2)', marginBottom: 12 }}>
                <div className="card-head">
                  <h2>有 {data.warnings.length} 条规则没能转换</h2>
                </div>
                <ul className="warn-list">
                  {data.warnings.slice(0, 20).map((w, i) => (
                    <li key={i}>{w}</li>
                  ))}
                  {data.warnings.length > 20 && <li>还有 {data.warnings.length - 20} 条…</li>}
                </ul>
              </div>
            )}
            <pre>{preview}</pre>
            <div className="row" style={{ marginTop: 10 }}>
              <span className="note">
                {data.filename}
                {SHARE_FORMATS.has(format) ? ' · 预览是 Base64 解码后的链接列表，下载/复制全文仍是 Base64 原文' : ''}
              </span>
              <div className="spacer" />
              <button className="btn btn-sm" onClick={() => copy(data.body, 'body')}>
                {copied === 'body' ? '已复制' : copied === 'fail' ? '复制失败' : '复制全文'}
              </button>
            </div>
          </>
        )}
      </div>

      <div className="card">
        <div className="card-head">
          <h2>关于规则顺序</h2>
        </div>
        <div className="note">
          上面这份配置里，<code className="mono">rules:</code> 的先后完全由「分组」页的策略组顺序决定，
          最后一条固定是 <code className="mono">MATCH,{state.settings.finalTarget || '最后一个组'}</code>。
          想确认位置对不对，直接在预览里搜关键词即可，不用再去猜。
        </div>
      </div>
    </>
  )
}
