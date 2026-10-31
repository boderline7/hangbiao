import { useState } from 'react'

export default function RuleSets({ state, mutate, toast }) {
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [behavior, setBehavior] = useState('domain')
  const [format, setFormat] = useState('yaml')
  const [interval, setIntervalValue] = useState('86400')

  const used = new Set()
  for (const r of state.rules) {
    if (String(r.type).toUpperCase() === 'RULE-SET') used.add(r.value)
  }

  const add = () => {
    if (!name.trim() || !url.trim()) {
      toast('名称和地址都要填', 'error')
      return
    }
    if (state.ruleSets.some((r) => r.name === name.trim())) {
      toast('同名规则集已存在', 'error')
      return
    }
    mutate((s) => {
      s.ruleSets.push({
        id: 'rs' + Math.random().toString(16).slice(2, 10),
        name: name.trim(),
        behavior,
        format,
        url: url.trim(),
        path: `./ruleset/${name.trim()}.${format}`,
        interval: Number(interval) || 86400,
      })
    })
    setName('')
    setUrl('')
  }

  return (
    <>
      <div className="card">
        <div className="card-head">
          <h2>规则集（rule-providers）</h2>
          <span className="hint">规则里用 RULE-SET,&lt;名称&gt;,&lt;目标&gt; 引用</span>
        </div>

        <div className="row" style={{ marginBottom: 14 }}>
          <input
            className="input"
            style={{ width: 150 }}
            placeholder="名称，如 anti-ad"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <input
            className="input"
            style={{ flex: 1, minWidth: 200 }}
            placeholder="规则集地址（https://…）"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
          <select className="select" value={behavior} onChange={(e) => setBehavior(e.target.value)}>
            <option value="domain">domain</option>
            <option value="ipcidr">ipcidr</option>
            <option value="classical">classical</option>
          </select>
          <select className="select" value={format} onChange={(e) => setFormat(e.target.value)}>
            <option value="yaml">yaml</option>
            <option value="text">text</option>
            <option value="mrs">mrs</option>
          </select>
          <input
            className="input"
            style={{ width: 100 }}
            value={interval}
            onChange={(e) => setIntervalValue(e.target.value)}
            title="更新间隔（秒）"
          />
          <button className="btn btn-primary" onClick={add}>
            添加
          </button>
        </div>

        {state.ruleSets.length === 0 ? (
          <div className="empty">还没有规则集。内置方案里的 anti-AD 示例会出现在这里。</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>名称</th>
                <th>类型</th>
                <th>地址</th>
                <th>间隔</th>
                <th>状态</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {state.ruleSets.map((rs) => (
                <tr key={rs.id}>
                  <td className="mono">{rs.name}</td>
                  <td>
                    <span className="badge">
                      {rs.behavior} · {rs.format}
                    </span>
                  </td>
                  <td style={{ maxWidth: 320, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {rs.url}
                  </td>
                  <td>{Math.round((rs.interval || 0) / 60)} 分</td>
                  <td>{used.has(rs.name) ? <span className="badge ok">被引用</span> : <span className="badge">未引用</span>}</td>
                  <td style={{ textAlign: 'right' }}>
                    <button
                      className="icon-btn danger"
                      onClick={() =>
                        mutate((s) => {
                          s.ruleSets = s.ruleSets.filter((x) => x.id !== rs.id)
                          s.rules = s.rules.filter((r) => !(String(r.type).toUpperCase() === 'RULE-SET' && r.value === rs.name))
                        })
                      }
                      title="删除（同时删除引用它的规则）"
                    >
                      ✕
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <div className="card-head">
          <h2>说明</h2>
        </div>
        <div className="note">
          规则集只在被规则引用时才会写进配置的 rule-providers。删掉引用它的规则，它就不会出现在生成的配置里。
          默认的 <code className="mono">antiad</code> 来自 ACL4SSR 的 <code className="mono">BanAD.list</code>（classical），
          预置但不引用：想用就在规则里加一条 <code className="mono">RULE-SET,antiad,&lt;目标组&gt;</code>。
        </div>
      </div>
    </>
  )
}
