package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"qingjin/internal/cpdaily"
	"qingjin/internal/store"
)

// SignRange 是服务端下发的签到范围。
type SignRange struct {
	TaskName        string  `json:"taskName"`
	SignInstanceWID string  `json:"signInstanceWid"`
	Lon             float64 `json:"lon"`
	Lat             float64 `json:"lat"`
	Radius          float64 `json:"radius"`
	Address         string  `json:"address"`
	BeginTime       string  `json:"beginTime"`
	EndTime         string  `json:"endTime"`
	// Applied 表示已写入账号
	Applied bool `json:"applied"`
}

// FetchSignRange 从服务端的查寝任务详情里读取签到点坐标与半径。
//
// 不依赖用户手动选点：学校设置的中心点和半径就是最稳妥的值，
// 直接用它们提交不会出现"范围外"。
func (s *Service) FetchSignRange(ctx context.Context, userID, accountID int64) (*SignRange, error) {
	account, err := s.db.AccountByID(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	if !account.HasCookie() {
		return nil, errors.New("账号尚未登录，请先登录")
	}

	client := s.clientFor(account)
	kinds := []cpdaily.TaskKind{cpdaily.KindAttendance}
	tasks, err := client.ListAllTasks(ctx, account.Host, kinds)
	if err != nil {
		return nil, fmt.Errorf("获取查寝任务失败：%w", err)
	}
	if len(tasks) == 0 {
		return nil, errors.New("当前没有查寝任务，无法获取签到范围")
	}

	// 优先选有签到点的任务
	var picked *cpdaily.Task
	var pickedDetail *cpdaily.TaskDetail
	for i := range tasks {
		t := tasks[i]
		detail, err := client.FetchDetail(ctx, account.Host, t.Kind, t)
		if err != nil {
			continue
		}
		if len(detail.SignPlaces) > 0 {
			picked = &t
			pickedDetail = detail
			break
		}
	}
	if picked == nil || pickedDetail == nil {
		return nil, errors.New("该任务是自由签到（无固定签到点），请手动在地图上选点")
	}

	place := pickedDetail.SignPlaces[0]
	result := &SignRange{
		TaskName:        pickedDetail.TaskName,
		SignInstanceWID: pickedDetail.SignInstanceWID,
		Lon:             place.Longitude.Float(),
		Lat:             place.Latitude.Float(),
		Radius:          place.Radius.Float(),
		Address:         firstNonEmptyStr(place.Address, place.PlaceName),
		BeginTime:       picked.StartTime,
		EndTime:         picked.EndTime,
	}
	return result, nil
}

// ApplySignRange 把服务端签到点写入账号，完成自动定位。
func (s *Service) ApplySignRange(ctx context.Context, userID, accountID int64) (*store.Account, *SignRange, error) {
	rng, err := s.FetchSignRange(ctx, userID, accountID)
	if err != nil {
		return nil, nil, err
	}
	account, err := s.db.AccountByID(ctx, userID, accountID)
	if err != nil {
		return nil, nil, err
	}
	account.Lon = rng.Lon
	account.Lat = rng.Lat
	if rng.Address != "" {
		account.Address = rng.Address
	}
	if err := s.db.UpdateAccount(ctx, account); err != nil {
		return nil, nil, err
	}
	rng.Applied = true
	return account, rng, nil
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
