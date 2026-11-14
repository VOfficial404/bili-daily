package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/VOfficial404/bili-daily/api"
)

func retentionReport(t *testing.T, e *Engine, mid string, started time.Time) *api.Run {
	t.Helper()
	run := &api.Run{RunSummary: api.RunSummary{ID: runID(started, mid), Mid: mid, Started: started, Ended: started.Add(time.Minute), Status: api.StatusOK}}
	if err := writeJSON(filepath.Join(e.runsDir(mid), run.ID+".json"), run); err != nil {
		t.Fatal(err)
	}
	return run
}

func requireRetentionReport(t *testing.T, e *Engine, run *api.Run, present bool) {
	t.Helper()
	_, err := os.Lstat(filepath.Join(e.runsDir(run.Mid), run.ID+".json"))
	if present && err != nil {
		t.Fatalf("report %s should remain: %v", run.ID, err)
	}
	if !present && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("report %s should be removed: %v", run.ID, err)
	}
}

func TestReportRetentionSettingsUpgradeAndValidation(t *testing.T) {
	v := newEnv(t)
	if got := v.e.Settings().ReportRetentionDays; got != 30 {
		t.Fatalf("default retention = %d", got)
	}
	if err := os.WriteFile(v.e.settingsFile(), []byte(`{"run_at":"07:45"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	upgraded := v.e.loadSettings()
	if upgraded.ReportRetentionDays != 30 || upgraded.RunAt != "07:45" {
		t.Fatalf("upgraded settings = %+v", upgraded)
	}
	for _, days := range []int{-1, 0, 366} {
		t.Run(strconv.Itoa(days), func(t *testing.T) {
			s := v.e.Settings()
			s.ReportRetentionDays = days
			if _, err := v.e.UpdateSettings(s); err == nil {
				t.Fatalf("accepted invalid retention %d", days)
			}
			if got := v.e.Settings().ReportRetentionDays; got != 30 {
				t.Fatalf("invalid update changed retention to %d", got)
			}
		})
	}
	for _, days := range []int{1, 365} {
		s := DefaultSettings()
		s.ReportRetentionDays = days
		if err := checkSettings(&s); err != nil {
			t.Fatalf("rejected boundary %d: %v", days, err)
		}
	}
}

func TestReportRetentionElapsedBoundaryAndSave(t *testing.T) {
	v := newEnv(t)
	v.login(42)
	// 使用非北京时间和纳秒精度，验证按经过的 24 小时计算而非按日期清理。
	v.now = time.Date(2026, 11, 14, 1, 2, 3, 123456789, time.FixedZone("other", -7*3600))
	cutoff := v.now.Add(-30 * 24 * time.Hour)
	old := retentionReport(t, v.e, "42", cutoff.Add(-time.Millisecond))
	boundary := retentionReport(t, v.e, "42", cutoff)
	recent := retentionReport(t, v.e, "42", cutoff.Add(time.Millisecond))
	fresh := &api.Run{RunSummary: api.RunSummary{ID: runID(v.now, "42"), Mid: "42", Started: v.now}}
	if err := v.e.saveRun(fresh); err != nil {
		t.Fatal(err)
	}
	requireRetentionReport(t, v.e, old, false)
	requireRetentionReport(t, v.e, boundary, true)
	requireRetentionReport(t, v.e, recent, true)
	requireRetentionReport(t, v.e, fresh, true)
}

func TestReportRetentionStartupAndIdleCleanup(t *testing.T) {
	v := newEnv(t)
	a := v.login(42)
	old := retentionReport(t, v.e, "42", v.now.Add(-31*24*time.Hour))
	recent := retentionReport(t, v.e, "42", v.now.Add(-29*24*time.Hour))
	if err := a.update(func(st *accountState) {
		st.LastRun = &old.RunSummary
		st.TodayDate = day(v.now)
		st.Today = map[string]api.TaskStatus{TaskDaily: {ID: TaskDaily, Status: api.StatusOK, Note: "保留今日结果"}}
	}); err != nil {
		t.Fatal(err)
	}
	// 没有会话的账号目录也需要清理，而不是只依赖正在排程的账号。
	orphan := retentionReport(t, v.e, "99", v.now.Add(-31*24*time.Hour))
	v.e.Stop()
	reopened, err := New(v.e.cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Stop)
	requireRetentionReport(t, reopened, old, false)
	requireRetentionReport(t, reopened, orphan, false)
	requireRetentionReport(t, reopened, recent, true)
	st := reopened.account("42").snapshot()
	if st.LastRun != nil || st.Today[TaskDaily].Note != "保留今日结果" {
		t.Fatalf("startup state = %+v", st)
	}
	var disk accountState
	if err := readJSON(reopened.stateFile("42"), &disk); err != nil || disk.LastRun != nil || disk.Today[TaskDaily].Status != api.StatusOK {
		t.Fatalf("persisted startup state = %+v, %v", disk, err)
	}
	// 小时循环调用同一个清理入口；推进引擎时钟，不创建新报告。
	v.now = v.now.Add(2 * 24 * time.Hour)
	reopened.pruneReports()
	requireRetentionReport(t, reopened, recent, false)
}

func TestReportRetentionReducedSettingsClearLastRun(t *testing.T) {
	v := newEnv(t)
	a := v.login(42)
	old := retentionReport(t, v.e, "42", v.now.Add(-8*24*time.Hour))
	fresh := retentionReport(t, v.e, "42", v.now.Add(-time.Hour))
	if err := a.update(func(st *accountState) {
		st.LastRun = &old.RunSummary
		st.Today = map[string]api.TaskStatus{TaskDaily: {ID: TaskDaily, Status: api.StatusOK}}
	}); err != nil {
		t.Fatal(err)
	}
	v.set(func(s *api.Settings) { s.ReportRetentionDays = 7 })
	requireRetentionReport(t, v.e, old, false)
	requireRetentionReport(t, v.e, fresh, true)
	st := a.snapshot()
	if st.LastRun != nil || st.Today[TaskDaily].Status != api.StatusOK {
		t.Fatalf("state after reducing retention = %+v", st)
	}
	var saved api.Settings
	if err := readJSON(v.e.settingsFile(), &saved); err != nil || saved.ReportRetentionDays != 7 {
		t.Fatalf("persisted retention = %d, %v", saved.ReportRetentionDays, err)
	}
}

func TestReportRetentionKeepsCountCap(t *testing.T) {
	v := newEnv(t)
	v.login(42)
	var oldest, newest *api.Run
	for i := range keepRuns + 5 {
		run := retentionReport(t, v.e, "42", v.now.Add(time.Duration(i-keepRuns-5)*time.Minute))
		if i == 0 {
			oldest = run
		}
		newest = run
	}
	if err := v.e.saveRun(newest); err != nil {
		t.Fatal(err)
	}
	if got := len(v.e.runIDs("42")); got != keepRuns {
		t.Fatalf("report count = %d, want %d", got, keepRuns)
	}
	requireRetentionReport(t, v.e, oldest, false)
	requireRetentionReport(t, v.e, newest, true)
}

func TestReportRetentionOnlyRemovesRecognizedRegularFiles(t *testing.T) {
	v := newEnv(t)
	v.login(42)
	old := retentionReport(t, v.e, "42", v.now.Add(-31*24*time.Hour))
	dir := v.e.runsDir("42")
	protected := []string{"notes.json", "20261399T999999999-42.json", runID(v.now.Add(-32*24*time.Hour), "99") + ".json", "." + old.ID + ".json-temp"}
	for _, name := range protected {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	directory := filepath.Join(dir, runID(v.now.Add(-33*24*time.Hour), "42")+".json")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	protected = append(protected, filepath.Base(directory))
	target := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(target, []byte("keep outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, runID(v.now.Add(-34*24*time.Hour), "42")+".json")
	if err := os.Symlink(target, link); err == nil {
		protected = append(protected, filepath.Base(link))
	} else {
		t.Logf("symlink creation unavailable: %v", err)
	}
	v.e.pruneReports()
	requireRetentionReport(t, v.e, old, false)
	for _, name := range protected {
		if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
			t.Errorf("protected entry %s removed: %v", name, err)
		}
	}
	if b, err := os.ReadFile(target); err != nil || string(b) != "keep outside" {
		t.Fatalf("outside target changed: %q, %v", b, err)
	}
}

func TestReportRetentionSkipsSymlinkDirectories(t *testing.T) {
	for _, location := range []string{"account", "runs"} {
		t.Run(location, func(t *testing.T) {
			v := newEnv(t)
			external := t.TempDir()
			oldID := runID(v.now.Add(-31*24*time.Hour), "42")
			var link, report string
			if location == "account" {
				link = v.e.accountDir("42")
				report = filepath.Join(external, "runs", oldID+".json")
			} else {
				link = v.e.runsDir("42")
				report = filepath.Join(external, oldID+".json")
			}
			if err := writeJSON(report, api.RunSummary{ID: oldID, Mid: "42"}); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(external, link); err != nil {
				t.Skipf("symlink creation unavailable: %v", err)
			}
			v.e.pruneReports()
			if _, err := os.Lstat(report); err != nil {
				t.Fatalf("removed report outside account data: %v", err)
			}
			if _, err := os.Lstat(link); err != nil {
				t.Fatalf("removed directory symlink: %v", err)
			}
		})
	}
}

func TestReportRetentionConcurrentSaveAndPrune(t *testing.T) {
	v := newEnv(t)
	v.login(42)
	var wg sync.WaitGroup
	errs := make(chan error, keepRuns+10)
	for i := range keepRuns + 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			started := v.now.Add(time.Duration(-i) * time.Minute)
			run := &api.Run{RunSummary: api.RunSummary{ID: runID(started, "42"), Mid: "42", Started: started}}
			errs <- v.e.saveRun(run)
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 5 {
			v.e.pruneReports()
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	ids := v.e.runIDs("42")
	if len(ids) != keepRuns || ids[0] != runID(v.now, "42") || ids[len(ids)-1] != runID(v.now.Add(-(keepRuns-1)*time.Minute), "42") {
		t.Fatalf("concurrent retention result = %v", ids)
	}
}

func TestReportRetentionClearsMissingLastRunWithoutRunsDirectory(t *testing.T) {
	v := newEnv(t)
	a := v.login(42)
	if err := a.update(func(st *accountState) {
		st.LastRun = &api.RunSummary{ID: runID(v.now.Add(-31*24*time.Hour), "42"), Mid: "42"}
		st.TodayDate = day(v.now)
		st.Today = map[string]api.TaskStatus{TaskDaily: {ID: TaskDaily, Status: api.StatusOK}}
	}); err != nil {
		t.Fatal(err)
	}
	v.e.pruneReports()
	st := a.snapshot()
	if st.LastRun != nil || st.TodayDate != day(v.now) || st.Today[TaskDaily].Status != api.StatusOK {
		t.Fatalf("state after clearing missing report = %+v", st)
	}
	var disk accountState
	if err := readJSON(v.e.stateFile("42"), &disk); err != nil || disk.LastRun != nil {
		t.Fatalf("persisted state = %+v, %v", disk, err)
	}
}
