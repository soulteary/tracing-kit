# tracing-kit

[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/tracing-kit.svg)](https://pkg.go.dev/github.com/soulteary/tracing-kit)
[![Go Report Card](https://goreportcard.com/badge/github.com/soulteary/tracing-kit)](https://goreportcard.com/report/github.com/soulteary/tracing-kit)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/soulteary/tracing-kit/graph/badge.svg)](https://codecov.io/gh/soulteary/tracing-kit)

[English](README.md)

一个轻量级的 Go 语言 OpenTelemetry 分布式追踪库。提供简单易用的 API，用于追踪上下文传播、Span 管理以及支持 OTLP 导出的 Tracer 初始化。

## 功能特性

- **Tracer 初始化** - 轻松设置带 OTLP HTTP 导出器的 OpenTelemetry Tracer
- **Span 管理** - 创建、配置和管理 Span 的简单 API
- **上下文传播** - 提取和注入追踪上下文，实现分布式追踪
- **属性支持** - 类型安全的 Span 属性设置方法
- **错误记录** - 记录错误并自动设置状态
- **测试辅助** - 使用内存导出器测试追踪代码的工具函数

## 安装

```bash
go get github.com/soulteary/tracing-kit
```

## 快速开始

### 初始化 Tracer

```go
import tracing "github.com/soulteary/tracing-kit"

func main() {
    // 使用 OTLP 端点初始化 Tracer
    tp, err := tracing.InitTracer("my-service", "v1.0.0", "localhost:4318")
    if err != nil {
        log.Fatal(err)
    }
    defer func() {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        tracing.Shutdown(ctx)
    }()

    // 检查追踪是否已启用
    if tracing.IsEnabled() {
        log.Println("追踪已启用")
    }
}
```

### 创建和管理 Span

```go
import (
    tracing "github.com/soulteary/tracing-kit"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
)

func processRequest(ctx context.Context) error {
    // 开始一个新的 Span
    ctx, span := tracing.StartSpan(ctx, "process.request")
    defer span.End()

    // 设置字符串属性
    tracing.SetSpanAttributes(span, map[string]string{
        "request.id":   "12345",
        "request.type": "api",
    })

    // 设置混合类型属性
    tracing.SetSpanAttributesFromMap(span, map[string]interface{}{
        "user.id":       42,
        "request.size":  int64(1024),
        "response.time": 0.125,
        "cached":        true,
    })

    // 模拟一些工作
    if err := doWork(ctx); err != nil {
        // 在 Span 上记录错误
        tracing.RecordError(span, err)
        return err
    }

    // 设置成功状态
    tracing.SetSpanStatus(span, codes.Ok, "请求处理成功")
    return nil
}

func doWork(ctx context.Context) error {
    // 创建子 Span
    ctx, span := tracing.StartSpan(ctx, "do.work", 
        trace.WithSpanKind(trace.SpanKindInternal))
    defer span.End()

    // 从上下文获取 Span
    currentSpan := tracing.GetSpanFromContext(ctx)
    currentSpan.AddEvent("工作开始")

    // ... 执行实际工作 ...

    return nil
}
```

### 追踪上下文传播

```go
import tracing "github.com/soulteary/tracing-kit"

// 从传入请求的 Header 中提取追踪上下文
func handleIncomingRequest(headers map[string]string) {
    ctx := tracing.ExtractTraceContext(context.Background(), headers)
    
    // 使用提取的上下文继续
    ctx, span := tracing.StartSpan(ctx, "handle.request")
    defer span.End()
    
    // 处理请求...
}

// 将追踪上下文注入到传出请求的 Header 中
func makeOutgoingRequest(ctx context.Context) {
    headers := make(map[string]string)
    tracing.InjectTraceContext(ctx, headers)
    
    // 使用 headers 发送 HTTP 请求
    // req.Header.Set("traceparent", headers["traceparent"])
}

// 分布式追踪的往返示例
func propagateTrace(ctx context.Context) {
    // 服务 A：注入上下文
    headers := make(map[string]string)
    tracing.InjectTraceContext(ctx, headers)
    
    // ... 发送请求到服务 B ...
    
    // 服务 B：提取上下文并继续追踪
    ctx = tracing.ExtractTraceContext(context.Background(), headers)
    ctx, span := tracing.StartSpan(ctx, "service.b.operation")
    defer span.End()
}
```

### HTTP 中间件示例

```go
import (
    tracing "github.com/soulteary/tracing-kit"
    "net/http"
)

func TracingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 从 Header 中提取追踪上下文
        headers := make(map[string]string)
        for k, v := range r.Header {
            if len(v) > 0 {
                headers[k] = v[0]
            }
        }
        ctx := tracing.ExtractTraceContext(r.Context(), headers)
        
        // 为此请求开始 Span
        ctx, span := tracing.StartSpan(ctx, r.Method+" "+r.URL.Path)
        defer span.End()
        
        // 设置请求属性
        tracing.SetSpanAttributes(span, map[string]string{
            "http.method": r.Method,
            "http.url":    r.URL.String(),
            "http.host":   r.Host,
        })
        
        // 使用带追踪的上下文继续
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

## API 参考

### Tracer 初始化

| 函数 | 描述 |
|------|------|
| `InitTracer(serviceName, version, endpoint)` | 使用 OTLP HTTP 导出器初始化 Tracer |
| `Shutdown(ctx)` | 优雅关闭 Tracer Provider |
| `GetTracer()` | 获取全局 Tracer（未初始化时返回 noop） |
| `IsEnabled()` | 检查追踪是否已启用 |

### Span 操作

| 函数 | 描述 |
|------|------|
| `StartSpan(ctx, name, opts...)` | 开始一个新的 Span |
| `SetSpanAttributes(span, attrs)` | 在 Span 上设置字符串属性 |
| `SetSpanAttributesFromMap(span, attrs)` | 在 Span 上设置混合类型属性 |
| `RecordError(span, err)` | 记录错误并设置错误状态 |
| `SetSpanStatus(span, code, description)` | 设置 Span 状态 |
| `GetSpanFromContext(ctx)` | 从上下文中获取 Span |

### 上下文传播

| 函数 | 描述 |
|------|------|
| `ExtractTraceContext(ctx, headers)` | 从 Header 中提取追踪上下文 |
| `InjectTraceContext(ctx, headers)` | 将追踪上下文注入 Header |

## 配置

Tracer 可以通过环境变量或编程方式配置：

| 环境变量 | 描述 | 默认值 |
|---------|------|--------|
| `OTEL_SERVICE_NAME` | 服务名称（被参数覆盖） | - |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP 端点（建议使用参数） | - |

使用环境检测的示例：

```go
tp, err := tracing.InitTracer(
    "my-service",
    "v1.0.0",
    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
)
```

## 测试覆盖率

本项目保持 100% 的测试覆盖率：

| 文件 | 覆盖率 |
|------|--------|
| tracing.go | 100% |
| span.go | 100% |
| propagation.go | 100% |
| test_helpers.go | 100% |
| **总计** | **100%** |

运行测试并查看覆盖率：

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

## 测试支持

该库提供了用于单元测试的辅助函数：

```go
import (
    tracing "github.com/soulteary/tracing-kit"
    "testing"
)

func TestMyTracedFunction(t *testing.T) {
    // 使用内存导出器设置测试 Tracer
    tp, exporter := tracing.SetupTestTracer(t)
    defer func() {
        tracing.ShutdownTracerProvider(tp)
        tracing.TeardownTestTracer()
    }()

    // 运行你的追踪代码
    ctx := context.Background()
    ctx, span := tracing.StartSpan(ctx, "test.operation")
    span.End()

    // 刷新并验证 Span
    tracing.ForceFlushTracerProvider(tp)
    spans := exporter.GetSpans()
    
    if len(spans) == 0 {
        t.Fatal("预期至少有一个 Span")
    }
}
```

## 环境要求

- Go 1.26 或更高版本
- OpenTelemetry Go SDK v1.39.0+

## 许可证

本项目采用 Apache License 2.0 许可证 - 详见 [LICENSE](LICENSE) 文件。

## 贡献

欢迎贡献！请随时提交 Pull Request。
