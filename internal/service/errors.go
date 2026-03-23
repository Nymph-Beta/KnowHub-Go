package service

import "errors"

// ErrServiceUnavailable 表示依赖未就绪，接口当前不可用。
var ErrServiceUnavailable = errors.New("service unavailable")
