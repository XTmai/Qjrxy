package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"qingjin/internal/cpdaily"
	"qingjin/internal/store"
)

// RunOptions 控制一次任务执行。
type RunOptions struct {
	UserID    int64
	AccountID int64
	TaskID    int64 // 0 表示自动挑选第一个待办
	Kind      string
	Source    string // manual / auto
	PlanID    int64
}

// RunOutcome 是执行结果。
type RunOutcome struct {
	LogID   int64  `json:"logId"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
	Task    string `json:"taskName"`
}

// RunTask 执行一次签到/查寝，并写入执行记录。
func (s *Service) RunTask(ctx context.Context, opts RunOptions) (*RunOutcome, error) {
	account, err := s.db.AccountByID(ctx, opts.UserID, opts.AccountID)
	if err != nil {
		return nil, err
	}
	if !account.HasCookie() {
		return nil, errors.New("账号尚未登录，请先登录或导入 Cookie")
	}
	// 设备 ID 为空时直接拒绝：拿空值提交等于用一台陌生设备去撞服务端，
	// 会被判定为「更换手机」并累加风控计数，必须先在账号里绑定真实设备。
	if strings.TrimSpace(account.DeviceID) == "" {
		return nil, errors.New("尚未绑定设备信息：请点「扫码自动获取」，用你平时打卡的那台手机扫二维码")
	}
	// 本项目只处理查寝（宿舍考勤）任务。
	// 二维码签到类任务涉及范围/时效差异较大，且容易产生无效提交，故不执行。
	if opts.Kind != "" && opts.Kind != string(cpdaily.KindAttendance) {
		return nil, errors.New("当前仅支持查寝任务，其他类型任务不会执行")
	}

	task, err := s.pickTask(ctx, account, opts)
	if err != nil {
		return nil, err
	}

	// 检查任务是否在可签到时段内。
	// 服务端在「任务还没开始」时不会报错，而是返回 code=0 但 signedStuInfo 为空，
	// 看起来像成功、实际没签上。提前拦住可以避免这类无效提交。
	if err := checkTaskWindow(task); err != nil {
		return nil, err
	}

	logID, err := s.db.CreateLog(ctx, &store.Log{
		UserID:    opts.UserID,
		AccountID: account.ID,
		PlanID:    opts.PlanID,
		TaskName:  task.TaskName,
		Kind:      task.Kind,
		Source:    defaultIfEmpty(opts.Source, "manual"),
		Status:    store.StatusExecuting,
		Message:   "正在提交",
	})
	if err != nil {
		return nil, err
	}

	photoPath := s.PhotoAbsPath(account.PhotoPath)
	hasPhoto := photoPath != "" && fileExists(photoPath)

	client := s.clientFor(account)
	// 坐标加入小幅随机偏移（约 ±50 米），避免每天同一坐标值过于规律；
	// 偏移量远小于签到点半径（通常 300~1500 米），不影响范围判定。
	lon, lat := jitterCoord(account.Lon, account.Lat)
	device := deviceProfile(account, lon, lat)
	signOpts := cpdaily.SignOptions{
		Kind: cpdaily.TaskKind(task.Kind),
		Task: cpdaily.Task{
			Kind:            cpdaily.TaskKind(task.Kind),
			SignInstanceWID: task.SignInstanceWID,
			SignWID:         task.SignWID,
			TaskName:        task.TaskName,
		},
		Lon:         lon,
		Lat:         lat,
		Position:    account.Address,
		Abnormal:    account.Abnormal,
		PhotoPath:   photoPath,
		UploadPhoto: hasPhoto,
	}

	result, detail, err := client.Sign(ctx, account.Host, account.TenantID, device, signOpts)
	if err != nil {
		detailJSON := marshalDetail(map[string]any{"error": err.Error()})
		_ = s.db.UpdateLog(ctx, logID, store.StatusPending, err.Error(), detailJSON)
		return &RunOutcome{LogID: logID, Status: store.StatusPending, Message: err.Error(),
			Task: task.TaskName}, nil
	}

	detailJSON := marshalDetail(buildLogDetail(detail, result, account, lon, lat))
	if result.Success {
		// 事后核对：服务端在「任务还没到签到时间」时也会返回 code=0 SUCCESS，
		// 靠响应字段无法区分。这里回查一次任务列表，确认该任务确实从
		//「未签到」里消失了，才算真成功。
		confirmed, checkErr := s.confirmSigned(ctx, account, task)
		if checkErr == nil && !confirmed {
			_ = s.db.UpdateLog(ctx, logID, store.StatusPending,
				"服务端返回成功，但任务仍在待办列表中", detailJSON)
			return &RunOutcome{
				LogID:   logID,
				Status:  store.StatusPending,
				Message: "服务端返回成功，但任务仍在待办列表中",
				Hint:    "可能是当前不在该任务的签到时段。请核对任务时间，到时间后再执行",
				Task:    task.TaskName,
			}, nil
		}

		_ = s.db.UpdateLog(ctx, logID, store.StatusSuccess, defaultIfEmpty(result.Message, "已完成"), detailJSON)
		_ = s.db.MarkTaskDone(ctx, account.ID, task.Kind, task.SignInstanceWID)
		return &RunOutcome{LogID: logID, Status: store.StatusSuccess,
			Message: defaultIfEmpty(result.Message, "已完成"), Task: task.TaskName}, nil
	}

	status := store.StatusFailed
	if isVerification(result.Code, result.Message) {
		status = store.StatusPending
	}
	_ = s.db.UpdateLog(ctx, logID, status, defaultIfEmpty(result.Message, "提交失败"), detailJSON)
	if cpdaily.IsAuthExpired(errors.New(result.Message)) {
		_ = s.db.UpdateAccountSession(ctx, account.ID, account.Cookies, "expired")
	}
	return &RunOutcome{LogID: logID, Status: status, Message: result.Message,
		Hint: result.Hint, Task: task.TaskName}, nil
}

// pickTask 选择要执行的任务。
func (s *Service) pickTask(ctx context.Context, account *store.Account, opts RunOptions) (*store.Task, error) {
	if opts.TaskID > 0 {
		task, err := s.db.TaskByID(ctx, opts.UserID, opts.TaskID)
		if err != nil {
			return nil, err
		}
		if task.AccountID != account.ID {
			return nil, errors.New("任务不属于该账号")
		}
		return task, nil
	}

	cached, err := s.db.TasksByAccount(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	if len(cached) == 0 {
		if _, err := s.SyncTasks(ctx, opts.UserID, account.ID); err != nil {
			return nil, err
		}
		if cached, err = s.db.TasksByAccount(ctx, account.ID); err != nil {
			return nil, err
		}
	}
	for i := range cached {
		t := cached[i]
		if t.Status == "done" {
			continue
		}
		// 只挑查寝任务
		if t.Kind != string(cpdaily.KindAttendance) {
			continue
		}
		if opts.Kind != "" && t.Kind != opts.Kind {
			continue
		}
		return &t, nil
	}
	return nil, errors.New("当前没有待处理的查寝任务")
}

// confirmSigned 回查任务列表，确认任务是否真的完成了。
//
// 返回 (true, nil) 表示已不在待办中（成功）；
// (false, nil) 表示仍在待办中（未真正签到）；
// 查询失败时返回 err，调用方应视为「无法确认」而不是失败。
func (s *Service) confirmSigned(ctx context.Context, account *store.Account, task *store.Task) (bool, error) {
	client := s.clientFor(account)
	list, err := client.ListAllTasks(ctx, account.Host, []cpdaily.TaskKind{cpdaily.KindAttendance})
	if err != nil {
		return false, err
	}
	for _, t := range list {
		if t.SignInstanceWID == task.SignInstanceWID {
			return false, nil // 还在待办里，说明没签上
		}
	}
	return true, nil
}

// buildLogDetail 组装可回溯的执行详情。
func buildLogDetail(detail *cpdaily.TaskDetail, result *cpdaily.SignResult, account *store.Account, lon, lat float64) map[string]any {
	payload := map[string]any{
		"code":    result.Code,
		"message": result.Message,
		"lon":     lon,
		"lat":     lat,
		"address": account.Address,
		"device":  account.DeviceID,
	}
	if detail != nil {
		payload["taskName"] = detail.TaskName
		payload["isPhoto"] = detail.IsPhoto.Int() == 1
		payload["isNeedExtra"] = detail.IsNeedExtra.Int() == 1
		check := detail.CheckRange(lon, lat)
		payload["inRange"] = check.InRange
		payload["places"] = check.Places
	}
	return payload
}

func marshalDetail(v any) string {
	buf, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(buf)
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// isVerification 判断失败是否属于"需要人工核对"而非确定失败。
func isVerification(code, message string) bool {
	joined := code + " " + message
	for _, kw := range []string{"人脸", "验证", "二维码", "扫码", "定位异常", "暂未"} {
		if strings.Contains(joined, kw) {
			return true
		}
	}
	return false
}

// checkTaskWindow 校验任务是否处于可签到时段。
//
// 依据列表接口返回的 signStatus：
//   - "2" 未到签到时间
//   - "3" 已过期
//   - "4" 已签到
// 空值表示该任务不带状态（如自由查寝），交给服务端判定。
func checkTaskWindow(task *store.Task) error {
	switch strings.TrimSpace(task.SignStatus) {
	case "2":
		when := ""
		if task.SignDate != "" {
			when = "（" + task.SignDate
			if task.BeginClock != "" {
				when += " " + task.BeginClock + "-" + task.EndClock
			}
			when += "）"
		}
		return fmt.Errorf("「%s」还没到签到时间%s，请到时间后再执行", task.TaskName, when)
	case "3":
		return fmt.Errorf("「%s」已过签到时间，无法补签", task.TaskName)
	case "4":
		return fmt.Errorf("「%s」已签到，无需重复执行", task.TaskName)
	}
	return nil
}

// jitterCoord 给坐标加 ±0.0005 度（约 50 米）的随机偏移。
//
// 目的不是规避定位校验，而是避免长期使用完全相同的坐标值，
// 让提交看起来更接近真实用户的手动定位。偏移远小于签到点半径，
// 不会导致范围判定失败；坐标为 0（未设置）时不偏移。
func jitterCoord(lon, lat float64) (float64, float64) {
	if lon == 0 && lat == 0 {
		return lon, lat
	}
	const span = 0.0005
	return lon + (rand.Float64()*2-1)*span, lat + (rand.Float64()*2-1)*span
}

// TodayLabel 返回用于展示的当前时间文本。
func TodayLabel(t time.Time) string {
	weekdays := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	return fmt.Sprintf("%d月%d日%s", int(t.Month()), t.Day(), weekdays[int(t.Weekday())])
}
