import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from './api.js'
import Groups from './pages/Groups.jsx'
import RuleSets from './pages/RuleSets.jsx'
import Subs from './pages/Subs.jsx'
import ExportPage from './pages/Export.jsx'
import SettingsPage from './pages/Settings.jsx'

const TABS = [
  { id: 'groups', label: '分组' },
  { id: 'rulesets', label: '规则集' },
  { id: 'subs', label: '订阅' },
  { id: 'export', label: '导出' },
  { id: 'settings', label: '设置' },
]

export default function App() {
  const [state, setState] = useState(null)
  const [tab, setTab] = useState('groups')
  const [toast, setToast] = useState(null)
  const [theme, setTheme] = useState(() => localStorage.getItem('td-theme') || 'dark')
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const timer = useRef(null)
  const stateRef = useRef(null)

  // 让 stateRef 始终指向最新的 state，供保存逻辑读取。
  useEffect(() => {
    stateRef.current = state
  }, [state])

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('td-theme', theme)
  }, [theme])

  const showToast = useCallback((msg, kind = 'ok') => {
    setToast({ msg, kind })
    window.clearTimeout(showToast._t)
    showToast._t = window.setTimeout(() => setToast(null), kind === 'error' ? 5200 : 2400)
  }, [])

  const load = useCallback(async () => {
    try {
      setState(await api.state())
    } catch (e) {
      showToast(e.message, 'error')
    }
  }, [showToast])

  useEffect(() => {
    load()
  }, [load])

  const mutate = useCallback(
    (recipe) => {
      const prev = stateRef.current
      if (!prev) return
      const next = JSON.parse(JSON.stringify(prev))
      recipe(next)
      stateRef.current = next
      setState(next)
      setDirty(true)
      window.clearTimeout(timer.current)
      timer.current = window.setTimeout(async () => {
        try {
          await api.save(next)
          setDirty(false)
        } catch (e) {
          showToast('自动保存失败：' + e.message, 'error')
        }
      }, 450)
    },
    [showToast],
  )

  // 手动保存：清掉防抖，立刻写盘，并用服务端回执作为反馈。
  const saveNow = useCallback(async () => {
    const cur = stateRef.current
    if (!cur) return
    setSaving(true)
    window.clearTimeout(timer.current)
    try {
      const saved = await api.save(cur)
      setState(saved)
      setDirty(false)
      showToast(
        `已保存到服务器：${saved.groups.length} 组 / ${saved.rules.length} 条规则 · ${new Date().toLocaleTimeString()}`
      )
    } catch (e) {
      showToast('保存失败：' + e.message, 'error')
    } finally {
      setSaving(false)
    }
  }, [showToast])

  // 刷新：丢弃本地未保存的改动，重新读取服务器状态。
  const reload = useCallback(async () => {
    await load()
    setDirty(false)
    showToast('已从服务器重新加载')
  }, [load, showToast])

  const replace = useCallback((next) => {
    setState(next)
    setDirty(false)
  }, [])

  if (!state) {
    return (
      <div className="app">
        <div className="card" style={{ marginTop: 80 }}>
          <div className="note">正在读取配置…</div>
        </div>
      </div>
    )
  }

  // nodeNames / nodeCount 由服务端按全局节点筛选（包含 / 不包含正则）算好返回。
  const nodeNames = state.nodeNames || []

  const counts = {
    nodes: state.nodeCount ?? nodeNames.length,
    groups: state.groups.length,
    rules: state.rules.length + 1,
  }

  return (
    <div className="app">
      <div className="topbar">
        <div className="topbar-inner">
          <div className="brand">
            <h1>{state.settings.title || '航标'}</h1>
            <span className="dim">策略组顺序即规则优先级</span>
            <div className="stats">
              <span>
                <b>{counts.nodes}</b> 节点
              </span>
              <span>
                <b>{counts.groups}</b> 组
              </span>
              <span>
                <b>{counts.rules}</b> 规则
              </span>
              <button
                className="btn btn-ghost btn-sm"
                onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
                title="切换主题"
              >
                {theme === 'dark' ? '浅色' : '深色'}
              </button>
              <button
                className="btn btn-primary btn-sm"
                onClick={saveNow}
                disabled={saving}
                title="立即写入服务器 data/state.json"
              >
                {saving ? '保存中…' : dirty ? '保存更改' : '保存'}
              </button>
              <button className="btn btn-ghost btn-sm" onClick={reload} title="丢弃未保存改动，重新读取服务器状态">
                刷新
              </button>
            </div>
          </div>
          <div className="tabs">
            {TABS.map((t) => (
              <button
                key={t.id}
                className={'tab' + (tab === t.id ? ' active' : '')}
                onClick={() => setTab(t.id)}
              >
                {t.label}
              </button>
            ))}
          </div>
        </div>
      </div>

      {tab === 'groups' && (
        <Groups state={state} mutate={mutate} replace={replace} toast={showToast} nodeNames={nodeNames} />
      )}
      {tab === 'rulesets' && <RuleSets state={state} mutate={mutate} toast={showToast} />}
      {tab === 'subs' && (
        <Subs state={state} replace={replace} toast={showToast} nodeNames={nodeNames} />
      )}
      {tab === 'export' && <ExportPage state={state} />}
      {tab === 'settings' && <SettingsPage state={state} mutate={mutate} />}

      {toast && <div className={'toast' + (toast.kind === 'error' ? ' error' : '')}>{toast.msg}</div>}
    </div>
  )
}
