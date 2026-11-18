// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package service

import (
	"context"
	"errors"

	"qjrxy/internal/captcha"
	"qjrxy/internal/cpdaily"
	"qjrxy/internal/events"
)

// CaptchaPending 返回该用户当前仍在等待人工处理的验证码。
//
// 浏览器弹窗与刷新恢复都读这里：接口只回传展示所需的最少信息，
// 不含任何提交用的 code 或会话码。
func (s *Service) CaptchaPending(userID int64) []*captcha.Pending {
	return s.captchas.List(userID)
}

// SubmitCaptchaCells 回传用户点选的格子（1 基）。
func (s *Service) SubmitCaptchaCells(userID int64, token string, cells []int) error {
	return s.captchas.Submit(userID, token, cells)
}

// SkipCaptcha 让用户换一张。
func (s *Service) SkipCaptcha(userID int64, token string) error {
	return s.captchas.Skip(userID, token)
}

// CancelCaptcha 用户主动放弃。
func (s *Service) CancelCaptcha(userID int64, token string) error {
	return s.captchas.Cancel(userID, token)
}

// manualCaptcha 构造交给 cpdaily 的人工兜底回调。
//
// 只有手动触发的执行才会注入它：定时任务没有人守在屏幕前，
// 挂起一个 100 秒的等待只会白白拖住调度器。
func (s *Service) manualCaptcha(userID, accountID int64) cpdaily.ManualCaptchaFunc {
	return func(ctx context.Context, req cpdaily.ManualCaptchaRequest) ([]int, error) {
		p := s.captchas.Open(userID, accountID, captcha.Request{
			Name:  req.Name,
			Grid:  req.Grid,
			Cells: req.Cells,
		})
		// 让已经打开的面板立刻弹窗；推送没连上也无所谓，前端会轮询。
		s.notify(userID, events.TopicCaptcha)

		// 关键：不能用发起请求的 ctx 等待。用户刷新页面、切网络都会
		// 断开 run 请求，但任务本身不该因此失败——验证码还等着他点。
		// 这里脱掉取消信号，只留自己的超时。
		waitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), captcha.DefaultTTL)
		defer cancel()

		cells, err := s.captchas.Wait(waitCtx, p)
		switch {
		case err == nil:
			return cells, nil
		case errors.Is(err, captcha.ErrSkip):
			return nil, cpdaily.ErrManualCaptchaSkip
		case errors.Is(err, captcha.ErrCanceled):
			return nil, cpdaily.AbortManualCaptcha("已取消本次验证码")
		case errors.Is(err, captcha.ErrExpired):
			// 超时不等同于用户放弃，但同样不该自动重试：
			// 再弹一次只会让用户面对一张已经过期的图。
			return nil, cpdaily.AbortManualCaptcha("验证码等待超时，请重新执行一次")
		}
		return nil, err
	}
}
