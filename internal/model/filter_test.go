package model

import "testing"

func names(ps []Proxy) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Name())
	}
	return out
}

func TestNodeFilter(t *testing.T) {
	st := &State{
		Subs: []Subscription{{
			Nodes: []Proxy{
				{"name": "🇭🇰 香港 01", "type": "ss", "server": "a", "port": 1},
				{"name": "🇯🇵 日本 01", "type": "ss", "server": "b", "port": 1},
				{"name": "剩余流量：100 GB", "type": "ss", "server": "c", "port": 1},
			},
		}},
		Settings: Settings{FilterInclude: "香港|HK", FilterExclude: "剩余流量|套餐"},
	}
	if got := names(st.Nodes()); len(got) != 1 || got[0] != "🇭🇰 香港 01" {
		t.Errorf("筛选结果不对：%v", got)
	}

	// 只排除、包含留空 → 其余全部保留
	st.Settings.FilterInclude = ""
	st.Settings.FilterExclude = "剩余流量"
	if got := names(st.Nodes()); len(got) != 2 {
		t.Errorf("只排除时应保留 2 个，实际 %v", got)
	}

	// 非法正则忽略，不影响其它规则
	st.Settings.FilterInclude = ""
	st.Settings.FilterExclude = "["
	if got := names(st.Nodes()); len(got) != 3 {
		t.Errorf("非法正则应被忽略，实际 %v", got)
	}
}
