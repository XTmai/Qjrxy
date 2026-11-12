// Package captcha 管理「等待用户手点」的签到验证码会话。
//
// 自动识别不可用时（未配置识别服务、模型报错、九张图里没找到目标物），
// 签到流程会把九宫格与目标词交给本包挂起，浏览器轮询到后弹窗，
// 用户点完格子再回传。会话只存在内存里：它是一次签到的瞬时状态，
// 进程重启后重跑一次签到即可，不值得落库。
package captcha

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

// DefaultTTL 是等待用户的默认时长。服务端下发的验证码会话通常两分钟
// 内有效，这里留一点余量；超过就只能重新 create 一张。
const DefaultTTL = 100 * time.Second

var (
	// ErrNotFound 表示 token 不存在、已用掉或不属于该用户。
	ErrNotFound = errors.New("验证码会话不存在或已结束")
	// ErrExpired 表示会话超时。
	ErrExpired = errors.New("验证码等待超时，请重新执行一次")
	// ErrCanceled 表示用户主动放弃。
	ErrCanceled = errors.New("已取消本次验证码")
	// ErrSkip 表示用户要求换一张。
	ErrSkip = errors.New("换一张")
	// ErrNoSelection 表示提交时一张图都没选。
	ErrNoSelection = errors.New("请至少选择一张包含目标的图片")
)

// Request 是交给前端展示的内容。
type Request struct {
	// Name 是要找的目标物，如「自行车」。
	Name string
	// Grid 是 3×3 九宫格的 data URL（image/jpeg;base64）。
	// 直接给拼好的图而不是 9 个外链：图片服务器要求带 Cookie 与 Referer，
	// 浏览器直连大概率 403，也避免暴露学校的图片地址。
	Grid string
	// Cells 是格子总数，恒为 9，但按接口下发值渲染以免写死。
	Cells int
}

// Pending 是一次等待人工处理的验证码会话。
type Pending struct {
	Token     string    `json:"token"`
	AccountID int64     `json:"accountId"`
	Name      string    `json:"name"`
	Grid      string    `json:"grid"`
	Cells     int       `json:"cells"`
	ExpiresAt time.Time `json:"expiresAt"`

	userID int64
	ch     chan answer
	once   sync.Once // 只允许被 Submit / Skip / Cancel 结束一次
}

type answer struct {
	cells []int
	err   error
}

// Broker 维护当前进程内所有待处理会话。
type Broker struct {
	mu    sync.Mutex
	items map[string]*Pending
	ttl   time.Duration
	now   func() time.Time // 测试注入
}

// NewBroker 创建会话管理器。
func NewBroker(ttl time.Duration) *Broker {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Broker{items: map[string]*Pending{}, ttl: ttl, now: time.Now}
}

// Open 登记一次等待，返回可供前端轮询/回传的会话。
func (b *Broker) Open(userID, accountID int64, req Request) *Pending {
	cells := req.Cells
	if cells <= 0 {
		cells = 9
	}
	p := &Pending{
		Token:     newToken(),
		AccountID: accountID,
		Name:      req.Name,
		Grid:      req.Grid,
		Cells:     cells,
		ExpiresAt: b.now().Add(b.ttl),
		userID:    userID,
		ch:        make(chan answer, 1),
	}
	b.mu.Lock()
	// 顺带清理过期项，避免用户开了多个窗口后内存堆着死会话。
	now := b.now()
	for token, item := range b.items {
		if now.After(item.ExpiresAt) {
			delete(b.items, token)
		}
	}
	b.items[p.Token] = p
	b.mu.Unlock()
	return p
}

// Wait 阻塞到用户回传、取消或超时。返回后会话即从表里移除。
func (b *Broker) Wait(ctx context.Context, p *Pending) ([]int, error) {
	defer b.remove(p.Token, p)
	select {
	case a := <-p.ch:
		return a.cells, a.err
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, ErrExpired
		}
		return nil, ErrCanceled
	}
}

// Submit 回传用户选中的格子（1 基）。
func (b *Broker) Submit(userID int64, token string, cells []int) error {
	p, err := b.lookup(userID, token)
	if err != nil {
		return err
	}
	// 去重并按格子序号校验范围：越界的编号会让 validate 拿到空 code，
	// 表现为「验证码错误」，用户完全看不出是自己点错了。
	seen := make(map[int]bool, len(cells))
	out := make([]int, 0, len(cells))
	for _, c := range cells {
		if c < 1 || c > p.Cells || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	if len(out) == 0 {
		return ErrNoSelection
	}
	sort.Ints(out)
	return b.finish(p, answer{cells: out})
}

// Cancel 由用户主动放弃。
func (b *Broker) Cancel(userID int64, token string) error {
	p, err := b.lookup(userID, token)
	if err != nil {
		return err
	}
	return b.finish(p, answer{err: ErrCanceled})
}

// Skip 由用户要求换一张。调用方收到后应重新 create 一张再挂起。
func (b *Broker) Skip(userID int64, token string) error {
	p, err := b.lookup(userID, token)
	if err != nil {
		return err
	}
	return b.finish(p, answer{err: ErrSkip})
}

// List 返回该用户当前仍在等待的会话。页面刷新后靠它恢复弹窗，
// 避免「刷新一下验证码就永远等不到了」。
func (b *Broker) List(userID int64) []*Pending {
	now := b.now()
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]*Pending, 0, len(b.items))
	for _, p := range b.items {
		if p.userID != userID || now.After(p.ExpiresAt) {
			continue
		}
		out = append(out, p)
	}
	// 按截止时间升序：多账号同时要验证码时，先处理更早到期的那个。
	sort.Slice(out, func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
	return out
}

func (b *Broker) lookup(userID int64, token string) (*Pending, error) {
	b.mu.Lock()
	p, ok := b.items[token]
	b.mu.Unlock()
	// 不区分「不存在」与「属于别人」，避免拿别人的 token 探测。
	if !ok || p.userID != userID {
		return nil, ErrNotFound
	}
	if b.now().After(p.ExpiresAt) {
		return nil, ErrExpired
	}
	return p, nil
}

func (b *Broker) finish(p *Pending, a answer) error {
	done := false
	p.once.Do(func() {
		p.ch <- a
		done = true
	})
	if !done {
		return ErrNotFound
	}
	return nil
}

func (b *Broker) remove(token string, p *Pending) {
	b.mu.Lock()
	if cur, ok := b.items[token]; ok && cur == p {
		delete(b.items, token)
	}
	b.mu.Unlock()
}

func newToken() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand 失败意味着系统已经不可用，这里不能退回时间戳
		// （token 会被猜到），直接 panic 更诚实。
		panic("captcha: 生成会话 token 失败：" + err.Error())
	}
	return hex.EncodeToString(buf[:])
}
