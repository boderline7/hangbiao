package scheme

import "testing"

func TestDetectRegion(t *testing.T) {
	cases := map[string]string{
		"🇭🇰 香港 01":       "HK",
		"HK-02":          "HK",
		"🇯🇵 日本 IPLC":     "JP",
		"美国 洛杉矶":         "US",
		"🇸🇬 狮城":          "SG",
		"台湾 新北":          "TW",
		"韩国 首尔":          "KR",
		"🇸🇪 瑞典":          OtherRegionsName,
		"企业专线":           "",
		"My Node":        "", // MY 是马来西亚码，但小写不算
		"Contact us":     "",
		"US Premium":     "US",
		"Netherlands DE": "DE",
	}
	for name, want := range cases {
		if got := DetectRegion(name); got != want {
			t.Errorf("DetectRegion(%q)=%q，期望 %q", name, got, want)
		}
	}
}
