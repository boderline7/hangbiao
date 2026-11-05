import { useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../api.js'

// 触屏设备上 HTML5 draggable 基本不生效（iOS 长按还会弹原生拖拽），
// 这类设备走 pointer 拖拽，并额外给上移/下移按钮保底。
const COARSE_POINTER =
  typeof window !== 'undefined' && window.matchMedia ? window.matchMedia('(pointer: coarse)').matches : false

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
  const [presetId, setPresetId] = useState('')
  const [syncing, setSyncing] = useState(false)
  const [adding, setAdding] = useState(false)
  const [newName, setNewName] = useState('')
  const [newType, setNewType] = useState('select')
  const [dragIndex, setDragIndex] = useState(null)
  const [overIndex, setOverIndex] = useState(null)

  const loadPresets = () =>
    api
      .presets()
      .then((list) => {
        const items = list || []
        setPresets(items)
        // 方案列表只包含已同步成功的方案；没有时下拉为空，不保留旧值。
        setPresetId((cur) => (items.some((p) => p.id === cur) ? cur : items[0] ? items[0].id : ''))
        return items
      })
      .catch(() => [])

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
    if (!presetId) {
      toast('请先点「同步 ACL4SSR」并成功找到方案', 'error')
      return
    }
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
          <select
            className="select"
            value={presetId}
            onChange={(e) => setPresetId(e.target.value)}
            disabled={presets.length === 0}
          >
            {presets.length === 0 && <option value="">尚未同步</option>}
            {presets.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name} · {p.groups} 组 / {p.rules} 条
              </option>
            ))}
          </select>
          <button className="btn" onClick={applyPreset} disabled={!presetId}>
            载入
          </button>
        </div>
        {presets.length === 0 ? (
          <div className="empty">
            还没有同步到 ACL4SSR，所以这里不显示任何方案与分组。点「同步 ACL4SSR」成功之后才会出现，
            避免把没同步下来的分组导出给客户端。
          </div>
        ) : (
          <div className="note">
            已同步 ACL4SSR{' '}
            {presets[0].revision ? presets[0].revision.slice(0, 7) : ''}
            {presets[0].fetchedAt ? ` · 同步于 ${presets[0].fetchedAt}` : ''}。载入后，列表里的先后顺序
            就是客户端的匹配优先级：靠上的组先命中。把「广告拦截」拖到「全球直连」上面，它的规则就会排在
            CN 直连之前；拖到最下面则几乎不会生效。
          </div>
        )}
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
                ruleSets={state.ruleSets}
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

function GroupBody({ group, groups, rules, nodeNames, mutate, toast, ruleSets }) {
  const [type, setType] = useState('DOMAIN-SUFFIX')
  const [value, setValue] = useState('')
  const [target, setTarget] = useState('')
  const [site, setSite] = useState('')
  const [dragMember, setDragMember] = useState(null)
  const [overMember, setOverMember] = useState(null)
  // 触屏拖拽状态：HTML5 的 draggable 在手机上不生效，这里用 pointer 事件补一套。
  const touchDrag = useRef({ active: false, pointerId: 0, from: 0, to: 0, moved: false })
  const suppressTap = useRef(false)

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

  const setFilter = (field, value) =>
    mutate((s) => {
      const g = s.groups.find((x) => x.id === group.id)
      if (g) g[field] = value
    })

  // 与后端保持一致：每行一条正则，非法正则直接忽略；先包含后排除。
  // 筛选只作用于节点（"全部节点"展开的与手选的都算），DIRECT/REJECT/策略组名不受影响。
  // 界面按同一规则实时隐藏被筛掉的节点，导出时后端也按此过滤。
  const filterRules = useMemo(() => {
    const parse = (text) =>
      (text || '')
        .split('\n')
        .map((x) => x.trim())
        .filter(Boolean)
        .map((x) => {
          try {
            return new RegExp(x)
          } catch {
            return null
          }
        })
        .filter(Boolean)
    const inc = parse(group.filterInclude)
    const exc = parse(group.filterExclude)
    const allows = (n) => {
      if (inc.length && !inc.some((re) => re.test(n))) return false
      if (exc.some((re) => re.test(n))) return false
      return true
    }
    const nodeSet = new Set(nodeNames)
    let hidden = 0
    for (const m of group.members || []) {
      if (nodeSet.has(m) && !allows(m)) hidden++
    }
    return { allows, nodeSet, hidden }
  }, [group.members, group.filterInclude, group.filterExclude, nodeNames])

  const moveMember = (from, to) => {
    if (from === null || to === null || from === to) return
    setMembers((list) => {
      const next = [...list]
      const [item] = next.splice(from, 1)
      next.splice(to, 0, item)
      return next
    })
  }

  // 触屏保底：点 ↑/↓ 交换相邻成员
  const moveMemberBy = (i, dir) => {
    setMembers((list) => {
      const to = i + dir
      if (to < 0 || to >= list.length) return list
      const next = [...list]
      const tmp = next[i]
      next[i] = next[to]
      next[to] = tmp
      return next
    })
  }

  const onChipPointerDown = (e, i) => {
    // 鼠标沿用 HTML5 draggable，触屏/手写笔走下面的 pointer 拖拽。
    if (e.pointerType === 'mouse') return
    // 按到 ✕ / 上下移按钮时不要触发拖拽
    if (e.target.closest && e.target.closest('button')) return
    suppressTap.current = false
    touchDrag.current = { active: true, pointerId: e.pointerId, from: i, to: i, moved: false }
    setDragMember(i)
    setOverMember(i)
    try {
      e.currentTarget.setPointerCapture(e.pointerId)
    } catch {
      // 个别浏览器不支持捕获，忽略即可
    }
  }

  const onChipPointerMove = (e) => {
    const t = touchDrag.current
    if (!t.active || e.pointerId !== t.pointerId) return
    e.preventDefault()
    const el = document.elementFromPoint(e.clientX, e.clientY)
    const chip = el && el.closest ? el.closest('.chip[data-idx]') : null
    if (!chip) return
    const idx = Number(chip.getAttribute('data-idx'))
    if (!Number.isInteger(idx)) return
    if (idx !== t.to) {
      t.to = idx
      setOverMember(idx)
    }
    if (idx !== t.from) t.moved = true
  }

  const onChipPointerEnd = (e, commit) => {
    const t = touchDrag.current
    if (!t.active || e.pointerId !== t.pointerId) return
    t.active = false
    try {
      e.currentTarget.releasePointerCapture(t.pointerId)
    } catch {
      // 忽略
    }
    if (commit && t.moved) moveMember(t.from, t.to)
    suppressTap.current = t.moved
    setDragMember(null)
    setOverMember(null)
  }

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
        <div className="section-label">
          成员（拖动或点 ↑↓ 调整顺序{group.type === 'select' ? '，第一个成员即客户端默认选中项' : ''}）
        </div>
        <div className="chips">
          {(group.members || [])
            .map((m, i) => ({ m, i }))
            .filter(({ m }) => !(filterRules.nodeSet.has(m) && !filterRules.allows(m)))
            .map(({ m, i }) => (
            <span
              className={
                'chip' +
                (dragMember === i ? ' dragging' : '') +
                (overMember === i && dragMember !== null && dragMember !== i ? ' drop-target' : '')
              }
              key={m + ':' + i}
              data-idx={i}
              draggable={!COARSE_POINTER}
              onPointerDown={(e) => onChipPointerDown(e, i)}
              onPointerMove={onChipPointerMove}
              onPointerUp={(e) => onChipPointerEnd(e, true)}
              onPointerCancel={(e) => onChipPointerEnd(e, false)}
              onDragStart={(e) => {
                setDragMember(i)
                setOverMember(i)
                e.dataTransfer.effectAllowed = 'move'
              }}
              onDragOver={(e) => {
                if (dragMember === null) return
                e.preventDefault()
                setOverMember(i)
              }}
              onDrop={(e) => {
                e.preventDefault()
                moveMember(dragMember, i)
                setDragMember(null)
                setOverMember(null)
              }}
              onDragEnd={() => {
                setDragMember(null)
                setOverMember(null)
              }}
              title="按住拖动调整顺序"
            >
              {group.type === 'select' && i === 0 && <span className="badge accent">默认</span>}
              <span>{m === '$nodes' ? '全部节点' : m === '$groups' ? '全部策略组' : m === '$regions' ? '全部地区组' : m}</span>
              {COARSE_POINTER && (
                <>
                  <button onClick={() => moveMemberBy(i, -1)} disabled={i === 0} title="上移">
                    ↑
                  </button>
                  <button
                    onClick={() => moveMemberBy(i, 1)}
                    disabled={i === (group.members || []).length - 1}
                    title="下移"
                  >
                    ↓
                  </button>
                </>
              )}
              <button
                onClick={() => {
                  // 刚拖动过就别当成点击，否则松手会误删成员
                  if (suppressTap.current) {
                    suppressTap.current = false
                    return
                  }
                  setMembers((list) => list.filter((x) => x !== m))
                }}
                title="移除"
              >
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
        <div className="section-label">节点筛选（每行一条正则，只作用于节点）</div>
        <div className="row" style={{ alignItems: 'stretch' }}>
          <textarea
            className="input"
            rows={2}
            style={{ flex: 1, minWidth: 180, resize: 'vertical' }}
            placeholder="包含：如 香港|HK，留空表示不限制"
            value={group.filterInclude || ''}
            onChange={(e) => setFilter('filterInclude', e.target.value)}
          />
          <textarea
            className="input"
            rows={2}
            style={{ flex: 1, minWidth: 180, resize: 'vertical' }}
            placeholder="不包含：如 过期|试用"
            value={group.filterExclude || ''}
            onChange={(e) => setFilter('filterExclude', e.target.value)}
          />
        </div>
        {filterRules.hidden > 0 && (
          <div style={{ fontSize: 12, color: 'var(--muted, #888)', marginTop: 6 }}>
            {`已按筛选隐藏 ${filterRules.hidden} 个节点（直连、策略组等不受影响）`}
          </div>
        )}
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
              {(ruleSets || []).map((rs) => (
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
