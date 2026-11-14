package api

import "encoding/json"

// MergeSettings 把用户在表单里改过的项（edited 相对 base 的差别）放到最新的设置 latest 上。
// 表单打开期间别的客户端改过的项不会被表单里的旧值盖掉。
func MergeSettings(base, edited, latest Settings) Settings {
	b, e, l := toMap(base), toMap(edited), toMap(latest)
	applyDiff(b, e, l)
	raw, _ := json.Marshal(l)
	var out Settings
	if err := json.Unmarshal(raw, &out); err != nil {
		return edited
	}
	return out
}

func toMap(s Settings) map[string]any {
	raw, _ := json.Marshal(s)
	var m map[string]any
	json.Unmarshal(raw, &m)
	return m
}

// applyDiff 把 edited 和 base 不同的叶子写进 dst；对象逐层比较，列表和标量整个替换
func applyDiff(base, edited, dst map[string]any) {
	for k, ev := range edited {
		bv := base[k]
		em, eok := ev.(map[string]any)
		bm, bok := bv.(map[string]any)
		if eok && bok {
			dm, ok := dst[k].(map[string]any)
			if !ok {
				dm = map[string]any{}
				dst[k] = dm
			}
			applyDiff(bm, em, dm)
			continue
		}
		if !jsonEqual(ev, bv) {
			dst[k] = ev
		}
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
