package middleware

import (
	"bytes"
	"io"
	"pai_smart_go_v2/pkg/log"
	"time"

	"github.com/gin-gonic/gin"
)

// BodyLogWriter 用于记录请求和响应的body
type BodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

// Write 实现了 io.Writer 接口，将响应写入 gin.ResponseWriter 和一个内部的 buffer
func (w *BodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// RequestLogger 作为gin.HandlerFunc，记录请求和响应的body
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// 读取并重新缓存请求体
		// 此处有问题需要之后重构
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
		}
		// 将读取的请求体重新设置回 c.Request.Body，以便后续处理函数可以正常读取
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

		// 使用自定义的 ResponseWriter 捕获响应
		blw := &BodyLogWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = blw

		// 继续处理请求
		c.Next()

		// 记录相关信息
		latency := time.Since(startTime)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		path := c.Request.URL.Path

		// 记录完整的请求和响应信息
		log.Infow("HTTP request",
			"latency", latency,
			"status", statusCode,
			"client_ip", clientIP,
			"method", method,
			"path", path,
			"request_body", string(requestBody),
			"response_body", blw.body.String(),
		)

	}
}
