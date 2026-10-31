import { useEffect, useMemo, useState } from 'react'
import { api } from '../api.js'

const RULE_TYPES = [
  'DOMAIN-SUFFIX',
  'DOMAIN',
  'DOMAIN-KEYWORD',
  'DOMAIN-REGEX',
  'IP-CIDR',
  'IP-CIDR6',
  'GEOIP',
  'GEOSITE',
  'DST-PORT',
  'PROCESS-NAME',
  'RULE-SET',
]

const GROUP_TYPES = [
  ['select', '手动选择'],
  ['url-test', '自动测速'],
  ['fallback', '故障转移'],
  ['load-balance', '负载均衡'],
]

function parseSite(input) {
  let s = (input || '').trim()
  if (!s) return null
  s = s.replace(/^[a-z]+:\/\//i, '')
  s = s.split('/')[0].split('?')[0].split('#')[0]
  s = s.replace(/:\d+$/, '')
  if (!s) return null
  if (/^\d{1,3}(\.\d{1,3}){3}(\/\d+)?$/.test(s)) {
    return { type: 'IP-CIDR', value: s.includes('/') ? s : s + '/32' }
  }
  return { type: 'DOMAIN-SUFFIX', value: s }
}

export default function Groups({ state, mutate, replace, toast, nodeNames }) {
  const [expanded, setExpanded] = useState(() => new Set())
  const [presets, setPresets] = useState([])
  const [presetId, setPresetId] = useState('standard')
  const [syncing, setSyncing] = useState(false)
  const [adding, setAdding] = useState(false)
  const [newName, setNewName] = useState('')
  const [newType, setNewType] = useState('select')
  const [dragIndex, setDragIndex] = useState(null)
  const [overIndex, setOverIndex] = useState(null)

  const loadPresets = () =>
    api
      .presets()
      .then((list) => setPresets(list || []))
      .catch(() => {})

  useEffect(() => {
    loadPresets()
  }, [])

  const groups = state.groups
  const rulesByGroup = useMemo(() => {
    const map = {}
    for (const g of groups) map[g.id] = []
    for (const r of state.rules) {
      if (map[r.position]) map[r.position].push(r)
    }
    return map
  }, [state.rules, groups])

  const toggle = (id) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const moveGroup = (from, to) => {
    if (from === null || to === null || from === to) return
    mutate((s) => {
      const [item] = s.groups.splice(from, 1)
      s.groups.splice(to, 0, item)
    })
  }

  const addGroup = () => {
    const name = newName.trim()
    if (!name) return
    if (groups.some((g) => g.name === name)) {
      toast('同名策略组已存在', 'error')
      return
    }
    mutate((s) => {
      s.groups.push({
        id: 'g' + Math.random().toString(16).slice(2, 10),
        name,
        type: newType,
        members: ['$nodes'],
        url: newType === 'select' ? '' : 'http://www.gstatic.com/generate_204',
        interval: newType === 'select' ? 0 : 300,
      })
    })
    setNewName('')
    setNewType('select')
    setAdding(false)
  }

  const removeGroup = (g) => {
    if (!window.confirm(`删除策略组「${g.name}」？组内规则会一起删除。`)) return
    mutate((s) => {
      s.groups = s.groups.filter((x) => x.id !== g.id)
      s.rules = s.rules.filter((r) => r.position !== g.id)
      if (s.settings.finalTarget === g.name) {
        s.settings.finalTarget = s.groups.length ? s.groups[s.groups.length - 1].name : 'DIRECT'
      }
    })
  }

  const syncPresets = async () => {
    setSyncing(true)
    try {
      const res = await api.syncPresets()
      await loadPresets()
      const extra = res && res.warnings && res.warnings.length ? `，${res.warnings.length} 条提示` : ''
      toast(`已同步 ACL4SSR ${res.revision.slice(0, 7)}${extra}`)
    } catch (e) {
      toast(e.message, 'error')
    } finally {
      setSyncing(false)
    }
  }

  const applyPreset = async () => {
    if (!window.confirm('载入方案会覆盖当前的分组、规则和规则集，订阅与节点保留。继续？')) return
    try {
      const next = await api.applyPreset(presetId)
      replace(next)
      setExpanded(new Set())
      toast('方案已载入')
    } catch (e) {
      toast(e.message, 'error')
    }
  }

  return (
    <>
      <div className="card">
        <div className="card-head">
          <h2>规则方案</h2>
          <span className="hint">规则与分组来自 ACL4SSR，可先载入再改</span>
          <div className="spacer" />
          <button className="btn" onClick={syncPresets} disabled={syncing}>
            {syncing ? '同步中…' : '同步 ACL4SSR'}
          </button>
          <select className="select" value={presetId} onChange={(e) => setPresetId(e.target.value)}>
            {presets.length === 0 && <option value="standard">标准</option>}
            {presets.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
                {p.source === 'upstream' ? '（上游同步）' : ''} · {p.groups} 组 / {p.rules} 条
              </option>
            ))}
          </select>
          <button className="btn" onClick={applyPreset}>
            载入
          </button>
        </div>
        <div className="note">
          列表里的先后顺序就是客户端的匹配优先级：靠上的组先命中。把「广告拦截」拖到「全球直连」上面，它的规则就会排在 CN
          直连之前；拖到最下面则几乎不会生效。
        </div>
      </div>

      <div className="card">
        <div className="card-head">
          <h2>策略组</h2>
          <span className="hint">拖左侧手柄排序</span>
          <div className="spacer" />
          <button className="btn btn-primary" onClick={() => setAdding((v) => !v)}>
            {adding ? '取消' : '新增策略组'}
          </button>
        </div>

        {adding && (
          <div className="row" style={{ marginBottom: 12 }}>
            <input
              className="input"
              style={{ flex: 1, minWidth: 160 }}
              placeholder="组名，例如 🛑 广告拦截"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && addGroup()}
              autoFocus
            />
            <select className="select" value={newType} onChange={(e) => setNewType(e.target.value)}>
              {GROUP_TYPES.map(([v, l]) => (
                <option key={v} value={v}>
                  {l}
                </option>
              ))}
            </select>
            <button className="btn btn-primary" onClick={addGroup}>
              创建
            </button>
          </div>
        )}

        {groups.length === 0 && <div className="empty">还没有策略组。先载入上面的方案，或点「新增策略组」。</div>}

        {groups.map((g, i) => (
          <div
            key={g.id}
            className={
              'group' +
              (dragIndex === i ? ' dragging' : '') +
              (overIndex === i && dragIndex !== null && dragIndex !== i ? ' drop-target' : '')
            }
            onDragOver={(e) => {
              if (dragIndex === null) return
              e.preventDefault()
              setOverIndex(i)
            }}
            onDrop={(e) => {
              e.preventDefault()
              moveGroup(dragIndex, overIndex)
              setDragIndex(null)
              setOverIndex(null)
            }}
          >
            <div className="group-head" onClick={() => toggle(g.id)}>
              <span
                className="handle"
                draggable
                onDragStart={(e) => {
                  setDragIndex(i)
                  setOverIndex(i)
                  e.dataTransfer.effectAllowed = 'move'
                }}
                onDragEnd={() => {
                  setDragIndex(null)
                  setOverIndex(null)
                }}
                onClick={(e) => e.stopPropagation()}
                title="拖动排序"
              >
                ⠿
              </span>
              <span className="order">{i + 1}</span>
              <span className="group-name">{g.name}</span>
              <span className="badge">{labelOfType(g.type)}</span>
              <span className="badge">{(g.members || []).length} 成员</span>
              <span className="badge accent">{(rulesByGroup[g.id] || []).length} 规则</span>
              <div className="spacer" />
              <button
                className="icon-btn danger"
                onClick={(e) => {
                  e.stopPropagation()
                  removeGroup(g)
                }}
                title="删除"
              >
                ✕
              </button>
            </div>

            {expanded.has(g.id) && (
              <GroupBody
                group={g}
                groups={groups}
                rules={rulesByGroup[g.id] || []}
                nodeNames={nodeNames}
                mutate={mutate}
                toast={toast}
              />
            )}
          </div>
        ))}
      </div>
    </>
  )
}

function labelOfType(t) {
  const hit = GROUP_TYPES.find(([v]) => v === t)
  return hit ? hit[1] : t || '手动选择'
}

function GroupBody({ group, groups, rules, nodeNames, mutate, toast }) {
  const [type, setType] = useState('DOMAIN-SUFFIX')
  const [value, setValue] = useState('')
  const [target, setTarget] = useState('')
  const [site, setSite] = useState('')

  const memberSet = new Set(group.members || [])
  const memberOptions = []
  if (!memberSet.has('$nodes')) memberOptions.push(['$nodes', '全部节点（动态）'])
  if (!memberSet.has('$groups')) memberOptions.push(['$groups', '全部策略组（动态）'])
  if (!memberSet.has('$regions')) memberOptions.push(['$regions', '全部地区组（动态）'])
  for (const [v, l] of [
    ['DIRECT', '直连 · DIRECT'],
    ['REJECT', '拒绝 · REJECT'],
    ['REJECT-DROP', '拒绝并断开 · REJECT-DROP'],
    ['PASS', '放行 · PASS'],
  ]) {
    if (!memberSet.has(v)) memberOptions.push([v, l])
  }
  for (const n of nodeNames) if (!memberSet.has(n)) memberOptions.push([n, n])
  for (const g of groups) if (g.name !== group.name && !memberSet.has(g.name)) memberOptions.push([g.name, '组 · ' + g.name])

  const setMembers = (fn) =>
    mutate((s) => {
      const g = s.groups.find((x) => x.id === group.id)
      if (g) g.members = fn(g.members || [])
    })

  const addRule = (rule) => {
    if (!rule.value) return
    mutate((s) => {
      s.rules.push({
        id: 'r' + Math.random().toString(16).slice(2, 10),
        type: rule.type,
        value: rule.value,
        target: rule.target || group.name,
        position: group.id,
        params: rule.params || '',
      })
    })
  }

  const moveRule = (ruleId, dir) => {
    mutate((s) => {
      const idxs = []
      s.rules.forEach((r, i) => {
        if (r.position === group.id) idxs.push(i)
      })
      const cur = s.rules.findIndex((r) => r.id === ruleId)
      const at = idxs.indexOf(cur)
      const swap = idxs[at + dir]
      if (swap === undefined) return
      const tmp = s.rules[cur]
      s.rules[cur] = s.rules[swap]
      s.rules[swap] = tmp
    })
  }

  return (
    <div className="group-body">
      <div>
        <div className="section-label">成员</div>
        <div className="chips">
          {(group.members || []).map((m) => (
            <span className="chip" key={m}>
              <span>{m === '$nodes' ? '全部节点' : m === '$groups' ? '全部策略组' : m === '$regions' ? '全部地区组' : m}</span>
              <button onClick={() => setMembers((list) => list.filter((x) => x !== m))} title="移除">
                ✕
              </button>
            </span>
          ))}
          <select
            className="select"
            style={{ padding: '2px 8px', fontSize: 12 }}
            value=""
            onChange={(e) => {
              if (!e.target.value) return
              const v = e.target.value
              setMembers((list) => [...list, v])
            }}
          >
            <option value="">+ 添加成员</option>
            {memberOptions.map(([v, l]) => (
              <option key={v} value={v}>
                {l}
              </option>
            ))}
          </select>
        </div>
      </div>

      <div>
        <div className="section-label">快捷加站点（自动放到本组）</div>
        <div className="row">
          <input
            className="input"
            style={{ flex: 1, minWidth: 180 }}
            placeholder="example.com 或 https://www.example.com/path"
            value={site}
            onChange={(e) => setSite(e.target.value)}
            onKeyDown={(e) => {
              if (e.key !== 'Enter') return
              const parsed = parseSite(site)
              if (!parsed) return
              addRule(parsed)
              setSite('')
            }}
          />
          <button
            className="btn"
            onClick={() => {
              const parsed = parseSite(site)
              if (!parsed) {
                toast('请输入域名或 IP', 'error')
                return
              }
              addRule(parsed)
              setSite('')
            }}
          >
            添加
          </button>
        </div>
      </div>

      <div>
        <div className="section-label">规则（组内顺序可调）</div>
        <div className="rules">
          {rules.length === 0 && <div className="empty">本组还没有规则</div>}
          {rules.map((r, i) => (
            <div className="rule" key={r.id}>
              <span className="type">{r.type}</span>
              <span className="value">{r.value}</span>
              <span className="target">→ {r.target}</span>
              <span className="row" style={{ gap: 2 }}>
                <button className="icon-btn" disabled={i === 0} onClick={() => moveRule(r.id, -1)} title="上移">
                  ↑
                </button>
                <button
                  className="icon-btn"
                  disabled={i === rules.length - 1}
                  onClick={() => moveRule(r.id, 1)}
                  title="下移"
                >
                  ↓
                </button>
              </span>
              <button
                className="icon-btn danger"
                onClick={() =>
                  mutate((s) => {
                    s.rules = s.rules.filter((x) => x.id !== r.id)
                  })
                }
                title="删除"
              >
                ✕
              </button>
            </div>
          ))}
        </div>

        <div className="row" style={{ marginTop: 8 }}>
          <select className="select" value={type} onChange={(e) => setType(e.target.value)}>
            {RULE_TYPES.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
          {type === 'RULE-SET' ? (
            <select
              className="select"
              style={{ flex: 1, minWidth: 140 }}
              value={value}
              onChange={(e) => setValue(e.target.value)}
            >
              <option value="">选择规则集…</option>
              {(state.ruleSets || []).map((rs) => (
                <option key={rs.id} value={rs.name}>
                  {rs.name}
                </option>
              ))}
            </select>
          ) : (
            <input
              className="input"
              style={{ flex: 1, minWidth: 140 }}
              placeholder="值，例如 google.com"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key !== 'Enter') return
                addRule({ type, value: value.trim(), target })
                setValue('')
              }}
            />
          )}
          <select className="select" value={target} onChange={(e) => setTarget(e.target.value)}>
            <option value="">目标：本组（{group.name}）</option>
            <option value="REJECT">目标：REJECT</option>
            <option value="DIRECT">目标：DIRECT</option>
            {groups
              .filter((g) => g.name !== group.name)
              .map((g) => (
                <option key={g.id} value={g.name}>
                  目标：{g.name}
                </option>
              ))}
          </select>
          <button
            className="btn"
            onClick={() => {
              if (!value.trim()) {
                toast('规则值不能为空', 'error')
                return
              }
              addRule({ type, value: value.trim(), target })
              setValue('')
            }}
          >
            添加规则
          </button>
        </div>
      </div>
    </div>
  )
}
