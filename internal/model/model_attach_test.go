package model

import (
	"reflect"
	"testing"
)

func TestAttachNodes(t *testing.T) {
	st := &State{
		Subs: []Subscription{{Nodes: []Proxy{{"name": "旧节点A"}, {"name": "旧节点B"}}}},
		Groups: []Group{
			{Name: "节点选择", Type: "select", Members: []string{"自动选择", "DIRECT", "旧节点A", "旧节点B"}},
			{Name: "自动", Type: "url-test", Members: []string{"旧节点A", "旧节点B"}},
			{Name: "香港", Type: "select", Members: []string{"旧节点A"}},
		},
	}
	before := st.NodeNames()
	st.Subs = append(st.Subs, Subscription{Nodes: []Proxy{{"name": "新节点C"}}})
	st.AttachNodes(before)

	want := map[string][]string{
		"节点选择": {"自动选择", "DIRECT", "旧节点A", "旧节点B", "新节点C"},
		"自动":   {"旧节点A", "旧节点B", "新节点C"},
		"香港":   {"旧节点A"},
	}
	for _, g := range st.Groups {
		if !reflect.DeepEqual(g.Members, want[g.Name]) {
			t.Errorf("「%s」成员 = %v，期望 %v", g.Name, g.Members, want[g.Name])
		}
	}
}

func TestAttachNodesNoChange(t *testing.T) {
	st := &State{
		Subs:   []Subscription{{Nodes: []Proxy{{"name": "节点A"}}}},
		Groups: []Group{{Name: "节点选择", Type: "select", Members: []string{"DIRECT", "节点A"}}},
	}
	before := st.NodeNames()
	st.AttachNodes(before)
	if got := st.Groups[0].Members; !reflect.DeepEqual(got, []string{"DIRECT", "节点A"}) {
		t.Errorf("没有新节点时不该改成员，实际 %v", got)
	}
}

func TestPruneMembers(t *testing.T) {
	st := &State{
		Subs: []Subscription{{Nodes: []Proxy{{"name": "节点A"}, {"name": "节点B"}}}},
		Groups: []Group{
			{Name: "节点选择", Type: "select", Members: []string{"自动选择", "DIRECT", "节点A", "节点B"}},
			{Name: "自动选择", Type: "url-test", Members: []string{"节点A", "节点B"}},
		},
	}
	// 模拟删除单节点：订阅里只剩节点A
	st.Subs = []Subscription{{Nodes: []Proxy{{"name": "节点A"}}}}
	st.PruneMembers()

	if got := st.Groups[0].Members; !reflect.DeepEqual(got, []string{"自动选择", "DIRECT", "节点A"}) {
		t.Errorf("节点选择成员 = %v，期望删掉已不存在的节点B", got)
	}
	if got := st.Groups[1].Members; !reflect.DeepEqual(got, []string{"节点A"}) {
		t.Errorf("自动选择成员 = %v，期望只剩节点A", got)
	}

	// 全部节点被删掉时，组不能空着
	st.Subs = []Subscription{}
	st.PruneMembers()
	if got := st.Groups[1].Members; !reflect.DeepEqual(got, []string{"DIRECT"}) {
		t.Errorf("节点全删后应兜底 DIRECT，实际 %v", got)
	}
}
