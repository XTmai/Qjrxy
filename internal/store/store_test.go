package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开测试库失败：%v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustUser(t *testing.T, db *DB) *User {
	t.Helper()
	u, err := db.CreateUser(context.Background(), "tester", "测试", "hash", 50)
	if err != nil {
		t.Fatalf("创建用户失败：%v", err)
	}
	return u
}

func TestCreateAndFetchUser(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	u, err := db.CreateUser(ctx, "alice", "爱丽丝", "hash1", 50)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 || u.Username != "alice" || u.Quota != 50 {
		t.Errorf("用户字段异常：%+v", u)
	}

	got, err := db.UserByUsername(ctx, "alice")
	if err != nil || got.ID != u.ID {
		t.Errorf("按用户名查询失败：%v", err)
	}

	if _, err := db.CreateUser(ctx, "alice", "", "hash2", 10); err == nil {
		t.Error("重复用户名应报错")
	}

	if _, err := db.UserByUsername(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的用户应返回 ErrNotFound，得到 %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := mustUser(t, db)

	if err := db.CreateSession(ctx, "tok1", u.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	got, err := db.SessionUser(ctx, "tok1")
	if err != nil || got.ID != u.ID {
		t.Fatalf("会话查询失败：%v", err)
	}

	if err := db.DeleteSession(ctx, "tok1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SessionUser(ctx, "tok1"); !errors.Is(err, ErrNotFound) {
		t.Error("注销后的会话应失效")
	}
}

func TestExpiredSession(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := mustUser(t, db)

	// 负数 TTL 表示已过期
	if err := db.CreateSession(ctx, "old", u.ID, -1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SessionUser(ctx, "old"); !errors.Is(err, ErrNotFound) {
		t.Error("过期会话应返回 ErrNotFound")
	}
}

func TestQuota(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u, _ := db.CreateUser(ctx, "quota", "", "h", 2)

	for i := 0; i < 2; i++ {
		ok, err := db.ConsumeQuota(ctx, u.ID)
		if err != nil || !ok {
			t.Fatalf("第 %d 次扣减应成功：%v %v", i+1, ok, err)
		}
	}
	ok, err := db.ConsumeQuota(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("超出额度不应扣减成功")
	}

	if err := db.GrantQuota(ctx, u.ID, 5); err != nil {
		t.Fatal(err)
	}
	updated, _ := db.UserByID(ctx, u.ID)
	if updated.Quota != 7 {
		t.Errorf("充值后总额度应为 7，得到 %d", updated.Quota)
	}
	if ok, _ := db.ConsumeQuota(ctx, u.ID); !ok {
		t.Error("充值后应可继续扣减")
	}
}

func TestAccountCRUD(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := mustUser(t, db)

	acc, err := db.CreateAccount(ctx, &Account{
		UserID: u.ID, SchoolName: "测试大学", Username: "20230001",
		Host: "test.campusphere.net", Lon: 116.4, Lat: 39.9,
	})
	if err != nil {
		t.Fatal(err)
	}
	if acc.ID == 0 || acc.LoginStatus != "unknown" {
		t.Errorf("账号默认值异常：%+v", acc)
	}
	if acc.DeviceModel == "" || acc.BuildID == "" {
		t.Error("设备指纹应有默认值")
	}

	// 同学校同学号不允许重复（返回可识别的 ErrDuplicateAccount，便于上层复用已有账号）
	if _, err := db.CreateAccount(ctx, &Account{
		UserID: u.ID, SchoolName: "测试大学", Username: "20230001", Host: "h",
	}); !errors.Is(err, ErrDuplicateAccount) {
		t.Errorf("重复账号应返回 ErrDuplicateAccount，实际：%v", err)
	}
	if found, err := db.AccountBySchoolUsername(ctx, u.ID, "测试大学", "20230001"); err != nil || found.ID != acc.ID {
		t.Errorf("按学校+学号反查失败：%v %+v", err, found)
	}
	if _, err := db.AccountBySchoolUsername(ctx, u.ID, "测试大学", "99999999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的学号应返回 ErrNotFound，实际：%v", err)
	}

	acc.Alias = "主账号"
	if err := db.UpdateAccount(ctx, acc); err != nil {
		t.Fatal(err)
	}
	got, err := db.AccountByID(ctx, u.ID, acc.ID)
	if err != nil || got.Alias != "主账号" {
		t.Errorf("更新未生效：%v %+v", err, got)
	}

	// 其他用户无法访问
	other, _ := db.CreateUser(ctx, "other", "", "h", 10)
	if _, err := db.AccountByID(ctx, other.ID, acc.ID); !errors.Is(err, ErrNotFound) {
		t.Error("跨用户访问应返回 ErrNotFound")
	}

	if err := db.DeleteAccount(ctx, u.ID, acc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AccountByID(ctx, u.ID, acc.ID); !errors.Is(err, ErrNotFound) {
		t.Error("删除后应查不到")
	}
}

func TestTasksReplaceAndMark(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := mustUser(t, db)
	acc, _ := db.CreateAccount(ctx, &Account{UserID: u.ID, SchoolName: "S", Username: "1", Host: "h"})

	if err := db.ReplaceTasks(ctx, acc.ID, []Task{
		{AccountID: acc.ID, Kind: "attendance", SignInstanceWID: "w1", TaskName: "晚间查寝"},
		{AccountID: acc.ID, Kind: "sign", SignInstanceWID: "w2", TaskName: "日常签到"},
	}); err != nil {
		t.Fatal(err)
	}

	tasks, err := db.TasksByAccount(ctx, acc.ID)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("任务数量异常：%v %d", err, len(tasks))
	}

	// 重复 WID 应被跳过而不是报错
	if err := db.ReplaceTasks(ctx, acc.ID, []Task{
		{AccountID: acc.ID, Kind: "attendance", SignInstanceWID: "w1", TaskName: "晚间查寝"},
		{AccountID: acc.ID, Kind: "attendance", SignInstanceWID: "w1", TaskName: "重复"},
	}); err != nil {
		t.Fatalf("重复任务不应报错：%v", err)
	}
	tasks, _ = db.TasksByAccount(ctx, acc.ID)
	if len(tasks) != 1 {
		t.Errorf("覆盖后应只剩 1 条，得到 %d", len(tasks))
	}

	if err := db.MarkTaskDone(ctx, acc.ID, "attendance", "w1"); err != nil {
		t.Fatal(err)
	}
	tasks, _ = db.TasksByAccount(ctx, acc.ID)
	if tasks[0].Status != "done" {
		t.Errorf("标记后状态应为 done，得到 %q", tasks[0].Status)
	}

	// ListTasks 只应看到自己的任务
	list, err := db.ListTasks(ctx, u.ID)
	if err != nil || len(list) != 1 {
		t.Errorf("用户任务列表异常：%v %d", err, len(list))
	}
}

func TestPlanCRUD(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := mustUser(t, db)
	acc, _ := db.CreateAccount(ctx, &Account{
		UserID: u.ID, Alias: "主账号", SchoolName: "S", Username: "1", Host: "h",
	})

	plan, err := db.CreatePlan(ctx, &Plan{
		UserID: u.ID, AccountID: acc.ID, Name: "查寝", Kind: "attendance",
		Mode: "custom", WindowStart: "20:00", WindowEnd: "23:00",
		Days: []int{0, 1, 2}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Days) != 3 {
		t.Errorf("星期未保存：%+v", plan.Days)
	}

	plans, err := db.ListPlans(ctx, u.ID)
	if err != nil || len(plans) != 1 {
		t.Fatalf("计划列表异常：%v %d", err, len(plans))
	}
	if plans[0].AccountAlias != "主账号" || plans[0].SchoolName != "S" {
		t.Errorf("计划应附带账号信息：%+v", plans[0])
	}

	if err := db.TogglePlan(ctx, u.ID, plan.ID, false); err != nil {
		t.Fatal(err)
	}
	enabled, err := db.ListEnabledPlans(ctx)
	if err != nil || len(enabled) != 0 {
		t.Errorf("停用后不应出现在启用列表：%v %d", err, len(enabled))
	}

	if err := db.RecordPlanRun(ctx, plan.ID, "已完成"); err != nil {
		t.Fatal(err)
	}
	plans, _ = db.ListPlans(ctx, u.ID)
	if plans[0].LastResult != "已完成" || plans[0].LastRunAt == "" {
		t.Errorf("执行结果未记录：%+v", plans[0])
	}

	if err := db.DeletePlan(ctx, u.ID, plan.ID); err != nil {
		t.Fatal(err)
	}
	if plans, _ := db.ListPlans(ctx, u.ID); len(plans) != 0 {
		t.Error("删除后列表应为空")
	}
}

func TestLogsAndStats(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := mustUser(t, db)
	acc, _ := db.CreateAccount(ctx, &Account{UserID: u.ID, SchoolName: "S", Username: "1", Host: "h"})

	ids := map[string]int64{}
	for _, status := range []string{StatusSuccess, StatusSuccess, StatusFailed, StatusPending} {
		id, err := db.CreateLog(ctx, &Log{
			UserID: u.ID, AccountID: acc.ID, TaskName: "查寝", Kind: "attendance",
			Source: "manual", Status: status,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids[status] = id
	}

	stats, err := db.TodayStats(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TodayTotal != 4 || stats.TodaySuccess != 2 || stats.TodayFailed != 1 || stats.TodayPending != 1 {
		t.Errorf("统计不符：%+v", stats)
	}

	logs, total, err := db.ListLogs(ctx, LogFilter{UserID: u.ID, Limit: 10})
	if err != nil || total != 4 || len(logs) != 4 {
		t.Fatalf("列表异常：%v total=%d len=%d", err, total, len(logs))
	}
	// 应按时间倒序，最新在前
	if logs[0].ID < logs[len(logs)-1].ID {
		t.Error("记录应按 ID 倒序")
	}

	failed, total, _ := db.ListLogs(ctx, LogFilter{UserID: u.ID, Status: StatusFailed})
	if total != 1 || len(failed) != 1 {
		t.Errorf("按状态筛选异常：total=%d", total)
	}

	if err := db.UpdateLog(ctx, ids[StatusExecuting], StatusSuccess, "ok", "{}"); err == nil {
		// ids[StatusExecuting] 为 0，更新 0 行也属于正常，这里只确认不 panic
	}

	one, err := db.LogByID(ctx, u.ID, ids[StatusSuccess])
	if err != nil || one.Status != StatusSuccess {
		t.Errorf("单条查询异常：%v %+v", err, one)
	}
	if _, err := db.LogByID(ctx, u.ID, 99999); !errors.Is(err, ErrNotFound) {
		t.Error("不存在的记录应返回 ErrNotFound")
	}
}

func TestCascadeDelete(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := mustUser(t, db)
	acc, _ := db.CreateAccount(ctx, &Account{UserID: u.ID, SchoolName: "S", Username: "1", Host: "h"})
	db.ReplaceTasks(ctx, acc.ID, []Task{{AccountID: acc.ID, Kind: "sign", SignInstanceWID: "w"}})
	db.CreatePlan(ctx, &Plan{UserID: u.ID, AccountID: acc.ID, Name: "p", Kind: "sign", Days: []int{0}})
	db.CreateLog(ctx, &Log{UserID: u.ID, AccountID: acc.ID, TaskName: "t", Kind: "sign", Status: StatusSuccess})

	if err := db.DeleteAccount(ctx, u.ID, acc.ID); err != nil {
		t.Fatal(err)
	}
	if tasks, _ := db.TasksByAccount(ctx, acc.ID); len(tasks) != 0 {
		t.Error("删除账号应级联删除任务")
	}
	if plans, _ := db.ListPlans(ctx, u.ID); len(plans) != 0 {
		t.Error("删除账号应级联删除计划")
	}
	if logs, _, _ := db.ListLogs(ctx, LogFilter{UserID: u.ID}); len(logs) != 0 {
		t.Error("删除账号应级联删除记录")
	}
}
