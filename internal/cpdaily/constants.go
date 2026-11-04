// Package cpdaily 实现今日校园（CPDaily / Campusphere）签到、查寝、收集等接口的协议层。
//
// 协议细节参考以下开源项目并做了整合（详见 README 致谢）：
//   - github.com/YVFENGXS/CpdailyHelper      提交加密、任务详情、位置校验
//   - github.com/maserpoassr/jrxyzuixingnx   getSecretKey 动态密钥、CAS/IAP 登录、OBS 上传
//   - github.com/2257929377/campus-device-id Cordova 设备 ID 获取页（/device.html）
package cpdaily

// AppVersion 当前模拟的今日校园客户端版本。
const AppVersion = "9.9.24"

// DefaultTenantID 兜底租户 ID（学校解析失败且未配置时使用）。
const DefaultTenantID = "dbbbb15d-e6ed-423b-8631-3b899640d1f8"

// DefaultBuildID Android 构建号。
const DefaultBuildID = "VLFCNXM"

// FKeyCat 签到 bodyString 的 AES-128-CBC 密钥。
// 由 getSecretKey 接口协商派生（interleave("h8r7uN8t" + catSecret)），
// 实测全平台固定；服务端轮换后可用 FetchSecretKey 重新获取。
const FKeyCat = "hru89rwr87Nthifa"

// 设备扩展头 DES 密钥。
const (
	desKeyExtension = "b3L26XNL"
	desKeyInfo      = "ST83=@XV"
	desKeyToken     = "XCE927=="
)

// TaskKind 任务类型。
type TaskKind string

const (
	KindSign       TaskKind = "sign"       // 日常签到
	KindAttendance TaskKind = "attendance" // 查寝 / 宿舍考勤
	KindCollect    TaskKind = "collect"    // 信息收集
)

// Label 返回中文名称。
func (k TaskKind) Label() string {
	switch k {
	case KindAttendance:
		return "查寝"
	case KindCollect:
		return "信息收集"
	default:
		return "签到"
	}
}

// Endpoints 一类任务所需的接口集合。
type Endpoints struct {
	List    string
	Detail  string
	Submit  string
	Submit2 string // 备用提交接口（新老协议并存时使用）
	STS     string
	OSS     string // 旧版上传策略
	Preview string
}

var endpointTable = map[TaskKind]Endpoints{
	KindSign: {
		List:    "/wec-counselor-sign-apps/stu/sign/getStuSignInfosInOneDay",
		Detail:  "/wec-counselor-sign-apps/stu/sign/detailSignInstance",
		Submit:  "/wec-counselor-sign-apps/stu/sign/submitSign",
		STS:     "/wec-counselor-sign-apps/stu/obs/getUploadPolicy",
		OSS:     "/wec-counselor-sign-apps/stu/obs/getUploadPolicy",
		Preview: "/wec-counselor-sign-apps/stu/sign/previewAttachment",
	},
	KindAttendance: {
		List:   "/wec-counselor-attendance-apps/student/attendance/getStuAttendacesInOneDay",
		Detail: "/wec-counselor-attendance-apps/student/attendance/detailSignInstance",
		Submit: "/wec-counselor-attendance-apps/student/attendance/submitSign",
		// 华为 OBS 上传策略（getStsAccess 已废弃，返回 404）
		STS:     "/wec-counselor-attendance-apps/student/attendance/getUploadPolicy",
		OSS:     "/wec-counselor-sign-apps/stu/obs/getUploadPolicy",
		Preview: "/wec-counselor-sign-apps/stu/sign/previewAttachment",
	},
	KindCollect: {
		List:    "/wec-counselor-collector-apps/stu/collector/queryCollectorProcessingList",
		Detail:  "/wec-counselor-collector-apps/stu/collector/detailCollector",
		Submit:  "/wec-counselor-collector-apps/stu/collector/submitForm",
		OSS:     "/wec-counselor-collector-apps/stu/obs/getUploadPolicy",
		Preview: "/wec-counselor-collector-apps/stu/collector/previewAttachment",
	},
}

// EndpointsFor 返回任务类型的接口集合。
func EndpointsFor(kind TaskKind) Endpoints {
	if e, ok := endpointTable[kind]; ok {
		return e
	}
	return endpointTable[KindSign]
}

// baseHeaders 查询类接口的公共请求头。
var baseHeaders = map[string]string{
	"Accept":          "application/json, text/plain, */*",
	"Accept-Encoding": "gzip,deflate",
	"Accept-Language": "zh-CN,en-US;q=0.8",
	"Content-Type":    "application/json",
	"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/83.0.4103.97 Safari/537.36",
}
