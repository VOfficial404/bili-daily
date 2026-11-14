package api

import (
	"slices"
	"testing"
)

func TestMergeSettingsKeepsChangesMadeElsewhere(t *testing.T) {
	base := Settings{RunAt: "09:30", Notify: Notify{Token: "old"}, Tasks: TaskSettings{Daily: DailyOptions{Coins: 5, UpIDs: []int64{1}}}}
	edited := base
	edited.RunAt = "07:00"
	edited.Tasks.Daily.UpIDs = []int64{2, 3}
	latest := base
	latest.Notify.Token = "new" // 别的客户端在表单打开期间改的
	latest.Tasks.Charge.UpID = 77

	got := MergeSettings(base, edited, latest)
	if got.RunAt != "07:00" || !slices.Equal(got.Tasks.Daily.UpIDs, []int64{2, 3}) {
		t.Errorf("表单的改动丢了：%+v", got)
	}
	if got.Notify.Token != "new" || got.Tasks.Charge.UpID != 77 {
		t.Errorf("别处的改动被旧值盖掉了：%+v", got)
	}
	if got.Tasks.Daily.Coins != 5 {
		t.Errorf("没改的项变了：%+v", got)
	}
}
