package bili

import "testing"

func TestLiveHeartbeatSignatureVectors(t *testing.T) {
	in := LiveHeartbeatInput{ParentID: 1, AreaID: 2, Sequence: 3, RoomID: 4, Buvid: "buvid", UUID: "uuid", ETS: 5, Interval: 6, TS: 7}
	// 手工排列 JSON 属性，再独立计算每轮 HMAC；中间值是十六进制文本。
	// MD5: a8934986302b22a0934143f02007d416
	// SHA1: bf400d47308ddf25dd6babaf0d4695fd44fe1032
	// SHA224: ca7510e27d99ee1ecc747b7772bc4bfc68d4f6ff7c32f67a4bdd4419
	// SHA512: 4c0ffdad976136966034a1f424b28ad050b1234306f0f0f2c099f5e750d7e1994cd83564b407221546b02c93a014d35088049916fd422b9fea14df05a58b7751
	for _, test := range []struct {
		rules []int
		want  string
	}{
		{[]int{0, 1, 2}, "62f34feb0771a45c8061d80e28e4292983a519a238f4105994fdb30d83a7961d"},
		{[]int{3, 4, 5}, "6e8f8b1ece0bfbaa7aa1414700f7de8f5aad82dae40fa564c192d2b110bfe86148294ab54a6ed2c37c6bdf2b4a812a45"},
		{nil, `{"platform":"web","parent_id":1,"area_id":2,"seq_id":3,"room_id":4,"buvid":"buvid","uuid":"uuid","ets":5,"time":6,"ts":7}`},
	} {
		got, err := LiveHeartbeatSign(in, "secret", test.rules)
		if err != nil || got != test.want {
			t.Errorf("规则 %v：签名 %q，错误 %v；应为 %q", test.rules, got, err, test.want)
		}
	}
	if _, err := LiveHeartbeatSign(in, "secret", []int{6}); err == nil {
		t.Fatal("没有拒绝未知的签名规则")
	}
}
