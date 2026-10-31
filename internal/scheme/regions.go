// Package scheme 里的地区组逻辑，对齐上游 Tower 的 NodeRegionResolver +
// ConfigurationGenerator.regionDefinitions：按节点名里的国旗/地区关键字
// 分出「🇭🇰 香港」这类地区组，每组再带一个隐藏的「· 延迟优选」自动测速组。
package scheme

import (
	"strings"
	"unicode"

	"towerdocker/internal/model"
)

// Region 地区定义，顺序与上游 regionDefinitions 一致。
type Region struct {
	Code     string
	Name     string
	Keywords []string
}

var Regions = []Region{
	{Code: "HK", Name: "🇭🇰 香港", Keywords: []string{"香港", "港", "hk", "hong kong", "hongkong"}},
	{Code: "JP", Name: "🇯🇵 日本", Keywords: []string{"日本", "东京", "大阪", "埼玉", "川日", "泉日", "沪日", "深日", "jp", "japan"}},
	{Code: "US", Name: "🇺🇸 美国", Keywords: []string{
		"美国", "美", "波特兰", "达拉斯", "俄勒冈", "凤凰城", "费利蒙", "硅谷",
		"拉斯维加斯", "洛杉矶", "圣何塞", "圣克拉拉", "西雅图", "芝加哥", "us", "united states"}},
	{Code: "SG", Name: "🇸🇬 新加坡", Keywords: []string{"新加坡", "狮城", "sg", "singapore"}},
	{Code: "TW", Name: "🇹🇼 台湾", Keywords: []string{"台湾", "新北", "彰化", "tw", "taiwan"}},
	{Code: "KR", Name: "🇰🇷 韩国", Keywords: []string{"韩国", "首尔", "kr", "korea", "kor"}},
	{Code: "GB", Name: "🇬🇧 英国", Keywords: []string{"英国", "uk", "gb", "britain", "伦敦", "london"}},
	{Code: "DE", Name: "🇩🇪 德国", Keywords: []string{"德国", "de", "germany", "法兰克福", "frankfurt"}},
	{Code: "FR", Name: "🇫🇷 法国", Keywords: []string{"法国", "fr", "france", "巴黎", "paris"}},
}

const (
	// OtherRegionsName 是「有国旗但不在上面 9 个地区」的节点去处。
	OtherRegionsName = "🌍 其他地区"
	// RegionAutoSuffix 是地区组里隐藏的测速组后缀，对应上游 automaticName。
	RegionAutoSuffix = " · 延迟优选"
	regionInterval   = 300
	regionTestURL    = "http://www.gstatic.com/generate_204"
)

// DetectRegion 返回节点所属地区的 Code；认不出返回空串。
// 国旗优先（机场已经明确声明），其次按关键字。两位国家码只在全大写时才算，
// 否则 "My Node" / "Contact us" 会被误判成马来西亚、美国（同上游规则）。
func DetectRegion(name string) string {
	if code := flaggedCode(name); code != "" {
		if _, ok := regionByCode(code); ok {
			return code
		}
		return OtherRegionsName
	}
	lower := strings.ToLower(name)
	for _, r := range Regions {
		for _, kw := range r.Keywords {
			if caseSensitiveCode(kw) {
				if hasUpperWord(name, strings.ToUpper(kw)) {
					return r.Code
				}
				continue
			}
			if strings.Contains(lower, kw) {
				return r.Code
			}
		}
	}
	return ""
}

// caseSensitiveCode 这两位码也是普通英文单词（us/my/de/fr…），必须全大写才采信。
func caseSensitiveCode(kw string) bool {
	if len(kw) != 2 {
		return false
	}
	switch kw {
	case "us", "de", "fr", "gb", "uk":
		return true
	}
	return false
}

func hasUpperWord(name, token string) bool {
	for _, f := range strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if f == token {
			return true
		}
	}
	return false
}

func regionByCode(code string) (Region, bool) {
	if code == "UK" {
		code = "GB"
	}
	for _, r := range Regions {
		if r.Code == code {
			return r, true
		}
	}
	return Region{}, false
}

// flaggedCode 取名字里前两个 regional-indicator 字符拼回国家码。
func flaggedCode(name string) string {
	var letters []rune
	for _, r := range name {
		if r >= 0x1F1E6 && r <= 0x1F1FF {
			letters = append(letters, rune('A'+r-0x1F1E6))
			if len(letters) == 2 {
				return string(letters)
			}
		}
	}
	return ""
}

// nodesByRegion 把节点按地区分组，键是地区组名（含「其他地区」）。
func nodesByRegion(nodes []model.Proxy) map[string][]string {
	out := map[string][]string{}
	for _, p := range nodes {
		code := DetectRegion(p.Name())
		if code == "" {
			continue
		}
		name := OtherRegionsName
		if r, ok := regionByCode(code); ok {
			name = r.Name
		}
		out[name] = append(out[name], p.Name())
	}
	return out
}

// RegionGroupNames 返回当前地区组（不含「· 延迟优选」）的有序名称。
func RegionGroupNames(st *model.State) []string {
	known := map[string]bool{OtherRegionsName: true}
	for _, r := range Regions {
		known[r.Name] = true
	}
	out := make([]string, 0, len(Regions)+1)
	for _, g := range st.Groups {
		if known[g.Name] && !strings.HasSuffix(g.Name, RegionAutoSuffix) {
			out = append(out, g.Name)
		}
	}
	return out
}

// SyncRegions 按当前节点重建地区组：先删掉旧的地区组，再按上游顺序追加。
// 每个地区同时生成「🇭🇰 香港」选择组和隐藏的「🇭🇰 香港 · 延迟优选」测速组。
// 运行时同步来的 ini 方案自带地区组，这里直接跳过。
func SyncRegions(st *model.State) {
	if !UsesBuiltinRegions(st) {
		return
	}
	// 「其他地区」也要连自动测速组一起清掉，否则每轮同步都会残留一份。
	known := map[string]bool{
		OtherRegionsName:                    true,
		OtherRegionsName + RegionAutoSuffix: true,
	}
	for _, r := range Regions {
		known[r.Name] = true
		known[r.Name+RegionAutoSuffix] = true
	}
	kept := st.Groups[:0]
	for _, g := range st.Groups {
		if !known[g.Name] {
			kept = append(kept, g)
		}
	}
	st.Groups = kept

	byRegion := nodesByRegion(st.Nodes())
	order := make([]string, 0, len(Regions)+1)
	for _, r := range Regions {
		order = append(order, r.Name)
	}
	order = append(order, OtherRegionsName)
	for _, name := range order {
		names := byRegion[name]
		if len(names) == 0 {
			continue
		}
		autoName := name + RegionAutoSuffix
		st.Groups = append(st.Groups,
			model.Group{
				ID:       model.NewID(),
				Name:     name,
				Type:     "select",
				Members:  append([]string{autoName}, names...),
				Interval: regionInterval,
			},
			model.Group{
				ID:       model.NewID(),
				Name:     autoName,
				Type:     "url-test",
				URL:      regionTestURL,
				Interval: regionInterval,
				Members:  names,
				Hidden:   true,
			},
		)
	}
}
