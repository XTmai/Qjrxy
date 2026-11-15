// Package buildinfo 存放产品自身的构建信息。
//
// 与 cpdaily.AppVersion 区分开：后者是「模拟今日校园客户端的版本号」（9.9.24），
// 用来构造请求头；这里是 Qjrxy 自己的版本，页脚、关于页、-version 都用它。
// 两者混用过一次，结果页面把客户端协议版本当成了产品版本显示。
package buildinfo

// Version 是产品版本号。发布时改这里，构建机也可用
// -ldflags "-X qjrxy/internal/buildinfo.Version=x.y.z" 覆盖。
var Version = "1.5.4"
