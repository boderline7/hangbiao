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
