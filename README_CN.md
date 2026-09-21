# tracing-kit

[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/tracing-kit/v2.svg)](https://pkg.go.dev/github.com/soulteary/tracing-kit/v2)
[![Go Report Card](.github/goreportcard.svg)](.github/goreportcard-report.md)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/soulteary/tracing-kit/graph/badge.svg)](https://codecov.io/gh/soulteary/tracing-kit)

[English](README.md)

一个轻量级的 Go 语言 OpenTelemetry 分布式追踪库：Span 辅助函数、基于普通 string map 的
追踪上下文传播，以及一行搞定的 OTLP 导出初始化。

## 包结构

| 包 | 依赖 | 用途 |
|----|------|------|
| `github.com/soulteary/tracing-kit/v2` | 仅 OpenTelemetry **API** | 创建 Span、传播上下文、安装 provider |
| `.../v2/otlp` | 额外引入 OpenTelemetry SDK、OTLP/HTTP 导出器、gRPC、protobuf | 在应用启动时初始化链路追踪 |
| `.../v2/tracingtest` | 额外引入 SDK 的 `tracetest` 导出器 | 测试带追踪的代码 |

根包既不 import SDK，也不 import 任何导出器。这正是 OpenTelemetry 自己要求的分层：
埋点代码只依赖 API，由应用来选择 SDK 和导出器。只创建 Span 的库，两者都不会链接进去。

实测数字 —— 只 import 根包的程序，与根包里还带着导出器的 v1.5.1 对比：

| | v1.5.1 | v2.0.0 |
|---|---:|---:|
| 二进制体积 | 18,095,419 B | 7,319,801 B（**−59.5%**） |
| 链接进来的非标准库包 | 191 | 42 |
| 参与构建的模块 | 22 | 8 |
| 你的 `go.mod` 里的 `// indirect` 依赖 | 21 | 7 |
| 你的 `go.sum` 里的模块 | 28 | 13 |

而 import `.../v2/otlp` 的程序，代价与 v1.5.1 完全一致。用到导出器才为它付费。

## 安装

```bash
go get github.com/soulteary/tracing-kit/v2
```

## 快速开始

### 初始化 Tracer

在应用启动时调用一次：

```go
import (
    "crypto/tls"
    "time"

    tracing "github.com/soulteary/tracing-kit/v2"
    "github.com/soulteary/tracing-kit/v2/otlp"
)

func main() {
    tp, err := otlp.InitTracerWithConfig(otlp.Config{
        ServiceName:    "my-service",
        ServiceVersion: "v1.0.0",
        Endpoint:       "collector.internal:4318",
        TLSConfig:      &tls.Config{MinVersion: tls.VersionTLS12},
        SampleRatio:    0.1,
        ExportTimeout:  10 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer func() {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        tp.Shutdown(ctx)
    }()

    if tracing.IsEnabled() {
        log.Println("链路追踪已启用")
    }
}
```

`Endpoint` 为空时，链路追踪被禁用，返回的 provider **什么都不记录**而不是 nil ——
因此 `defer tp.Shutdown(ctx)` 始终是安全的。

`otlp.InitTracer(name, version, endpoint)` 是旧的三参数形式。它保持原有行为——**明文导出，
且所有 span 全量采样**——因此现有调用方不受影响。任何会离开可信本地网络的场景，请使用
`InitTracerWithConfig`。

### 换用你自己的导出器

`tracing.Install` 接收一个接口 —— `Tracer` 加 `Shutdown` —— 所以任何 provider 都以同样的
方式安装，没有谁强迫你用 `otlp` 子包：

```go
exp, _ := stdouttrace.New()
tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))

tracing.Install(tp, "my-service")
tracing.SetDefaultPropagator()
defer tracing.Uninstall()
```

`SetDefaultPropagator` 单独成为一次调用是有意为之。OpenTelemetry 的全局 propagator 在被
设置之前什么都不传递；而一个自己不导出任何 span 的服务，仍然必须把收到的追踪上下文传给
下一跳 —— 所以即使链路追踪是关着的，也值得把传播打开。

### 创建和管理 Span

下面这些只需要根包，因此也是库代码应该写的样子：

```go
import (
    tracing "github.com/soulteary/tracing-kit/v2"
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

在有东西被安装之前，`GetTracer` 返回的是 noop tracer，因此这样写的代码在完全没有配置
链路追踪的进程里也能原样运行。

### 追踪上下文传播

```go
import tracing "github.com/soulteary/tracing-kit/v2"

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
```

### HTTP 中间件示例

```go
import (
    tracing "github.com/soulteary/tracing-kit/v2"
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

### 根包 —— `github.com/soulteary/tracing-kit/v2`

Span 操作：

| 函数 | 描述 |
|------|------|
| `StartSpan(ctx, name, opts...)` | 开始一个新的 Span |
| `SetSpanAttributes(span, attrs)` | 在 Span 上设置字符串属性 |
| `SetSpanAttributesFromMap(span, attrs)` | 在 Span 上设置混合类型属性 |
| `RecordError(span, err)` | 记录错误并设置错误状态；err 为 nil 时忽略 |
| `SetSpanStatus(span, code, description)` | 设置 Span 状态 |
| `GetSpanFromContext(ctx)` | 从上下文中获取 Span |

上下文传播：

| 函数 | 描述 |
|------|------|
| `SetDefaultPropagator()` | 安装 W3C trace context + baggage propagator |
| `ExtractTraceContext(ctx, headers)` | 从 Header 中提取追踪上下文 |
| `InjectTraceContext(ctx, headers)` | 将追踪上下文注入 Header |

已安装的 provider：

| 函数 | 描述 |
|------|------|
| `Install(p, serviceName)` | 进程级安装一个 `Provider`，并退休它替换掉的那一个 |
| `InstallDisabled(p, serviceName)` | 同上，但 `IsEnabled` 报告 false |
| `Uninstall()` | 卸下已安装的 provider，但不关闭它 |
| `Shutdown(ctx)` | 优雅关闭已安装的 provider |
| `GetTracer()` | 已安装的 Tracer（没有安装时返回 noop） |
| `IsEnabled()` | 是否安装了一个预期会导出的 provider |

`Provider` 就是 `trace.TracerProvider` 加上 `Shutdown(ctx) error`。
`*sdktrace.TracerProvider` 满足它。

### `.../v2/otlp`

| 函数 | 描述 |
|------|------|
| `InitTracerWithConfig(cfg)` | 从 `Config` 构建导出器和 provider 并安装 |
| `InitTracer(serviceName, version, endpoint)` | 三参数形式；明文导出，全量采样 |
| `NewExporter(ctx, cfg)` | 只要 OTLP/HTTP 导出器，provider 由你自己组装 |
| `Config.Sampler()` | 该 `Config` 所要求的采样器 |
| `Config.Resource(ctx)` | 该 `Config` 所要求的 resource |

### `.../v2/tracingtest`

| 函数 | 描述 |
|------|------|
| `Setup(t)` | 安装内存 Tracer；自行注册清理 |
| `Teardown()` | 再把它卸下 |
| `Shutdown(tp)` | 关闭 provider，忽略错误 |
| `ForceFlush(tp)` | 刷新 provider 的待发 Span，忽略错误 |

## 配置

```go
type Config struct {
    ServiceName    string      // 必填
    ServiceVersion string
    Endpoint       string      // 为空则禁用链路追踪
    Insecure       bool        // 明文 HTTP 导出
    TLSConfig      *tls.Config // Insecure 为 false 时使用；nil 表示系统默认
    SampleRatio    float64     // 0 表示 DefaultSampleRatio（0.1）
    SampleNone     bool        // 显式的"不采样"
    ExportTimeout  time.Duration
}
```

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `ServiceName` | — | 必填；导出的 resource 中会被 `OTEL_SERVICE_NAME` 覆盖 |
| `ServiceVersion` | 空 | 作为 service version 属性上报 |
| `Endpoint` | 空 | 为空则禁用链路追踪，返回一个什么都不记录的 provider |
| `Insecure` | `false` | 见下方警告 |
| `TLSConfig` | `nil` | `Insecure` 为 false 时，nil 表示系统默认 |
| `SampleRatio` | `DefaultSampleRatio`（0.1） | **零值表示默认值**，不是"不采样" |
| `SampleNone` | `false` | 显式表达"不采样"的方式 |
| `ExportTimeout` | SDK 默认 | 单次导出尝试的时限 |

### 传输安全

**`Insecure: true` 会以明文 HTTP 发送链路数据。** 链路里带着请求路径、用户标识、SQL 和
错误细节，因此在任何比 loopback 更宽的网络上明文导出，就等于把这些全部暴露。只在
collector 经由 loopback 或可信本地网络可达时使用它：

```go
// 开发环境
cfg := otlp.Config{ServiceName: "svc", Endpoint: "localhost:4318", Insecure: true}

// 生产环境
cfg = otlp.Config{
    ServiceName: "svc",
    Endpoint:    "collector.internal:4318",
    TLSConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
}
```

### 采样

采样器是 **`ParentBased`** 的，因此一条到达时已被采样的链路会在你的服务里继续保持被
采样——比率作用于本服务发起的链路，而不是它延续的 span。

`SampleRatio` 的零值表示 `DefaultSampleRatio`（10%），因此漏填这个字段不会静默改变行为。
要一条都不记录，请明说：

```go
cfg := otlp.Config{ServiceName: "svc", Endpoint: endpoint, SampleNone: true}
```

### 从环境变量读取端点

本库没有内置的环境变量处理，请自己读取：

```go
cfg := otlp.Config{
    ServiceName:    os.Getenv("OTEL_SERVICE_NAME"),
    ServiceVersion: buildVersion,
    Endpoint:       os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
    SampleRatio:    0.1,
}
```

`OTEL_SERVICE_NAME` 和 `OTEL_RESOURCE_ATTRIBUTES` 由 SDK 自己的 resource 探测读取，
而它在 `ServiceName` 之后生效，因此在 collector 看到的 resource 里是它说了算。
`ServiceName` 仍然用于命名 Tracer，也就是 instrumentation scope 里显示的那个名字。

## 测试覆盖率

在本仓库上用 `go test -race ./... -covermode=atomic` 实测：三个包**语句覆盖率均为 100%**。

```bash
go test ./... -coverprofile=coverage.out -covermode=atomic
go tool cover -func=coverage.out
```

## 测试支持

```go
import (
    "testing"

    tracing "github.com/soulteary/tracing-kit/v2"
    "github.com/soulteary/tracing-kit/v2/tracingtest"
)

func TestMyTracedFunction(t *testing.T) {
    // 使用内存导出器设置测试 Tracer。Setup 会自行注册清理，
    // 因此不需要任何 defer。
    //
    // 它接收 tracingtest.TestingT（Helper + Cleanup），*testing.T 满足
    // 该接口，因此自定义测试框架也可以传自己的实现。
    tp, exporter := tracingtest.Setup(t)

    // 运行你的追踪代码
    ctx, span := tracing.StartSpan(context.Background(), "test.operation")
    span.End()

    // 刷新并验证 Span
    tracingtest.ForceFlush(tp)
    spans := exporter.GetSpans()

    if len(spans) == 0 {
        t.Fatal("预期至少有一个 Span")
    }
}
```

## 升级说明（v2.0.0）

所有人的 import path 都要改，导出器和测试辅助函数从根包搬走。完整细节（包括这么做换来了
什么）见 [CHANGELOG.md](CHANGELOG.md)。

```go
// 改动前
import tracing "github.com/soulteary/tracing-kit"

tp, err := tracing.InitTracerWithConfig(tracing.Config{
    ServiceName:  "svc",
    OTLPEndpoint: endpoint,
})

// 改动后
import (
    tracing "github.com/soulteary/tracing-kit/v2"
    "github.com/soulteary/tracing-kit/v2/otlp"
)

tp, err := otlp.InitTracerWithConfig(otlp.Config{
    ServiceName: "svc",
    Endpoint:    endpoint,
})
```

| v1.5.1 | v2.0.0 |
|---|---|
| `tracing.InitTracer` | `otlp.InitTracer` |
| `tracing.InitTracerWithConfig` | `otlp.InitTracerWithConfig` |
| `tracing.Config` | `otlp.Config` |
| `tracing.Config.OTLPEndpoint` | `otlp.Config.Endpoint` |
| `tracing.DefaultSampleRatio` | `otlp.DefaultSampleRatio` |
| `tracing.SetupTestTracer` | `tracingtest.Setup` |
| `tracing.TeardownTestTracer` | `tracingtest.Teardown` |
| `tracing.ShutdownTracerProvider` | `tracingtest.Shutdown` |
| `tracing.ForceFlushTracerProvider` | `tracingtest.ForceFlush` |
| `tracing.TestingT` | `tracingtest.TestingT` |

根包里的其余部分 —— `StartSpan`、各个属性与状态辅助函数、`ExtractTraceContext`、
`InjectTraceContext`、`GetTracer`、`IsEnabled` 和 `Shutdown` —— 名字、签名和行为都不变。

没有保留任何兼容 shim。给搬走的初始化函数留一个 shim，就必须 import OTLP 导出器，
那会把 gRPC 和 protobuf 重新链接回每一个使用者，拆分的收益全部还回去。

## 升级说明（v1.5.1）

无需任何改动。`tracing.go` 里改了一行 import path；API、行为和依赖都没有变化。

- **语义约定改为 `semconv/v1.43.0`（此前 `v1.21.0`）。** 这个包比原来新了 22 个规范
  版本，也是本模块已经依赖的 `go.opentelemetry.io/otel` v1.46.0 里自带的当前版本 ——
  所以 `go.mod` 和 `go.sum` 完全没有变动，不会多下载任何东西。
- **导出的 resource 与原来逐字节一致。** 本模块用到的两个 helper —— `ServiceName` 和
  `ServiceVersion` —— 在两个版本里签名相同，返回的 key 也相同：`service.name` 和
  `service.version`。`InitTracer` 原本就没有设置 schema URL，现在也没有：resource 由
  `resource.WithAttributes` 和 `resource.WithFromEnv` 构建，所以改动前后
  `SchemaURL()` 都是 `""`。你的 collector 看到的数据没有任何差别。
- **环境要求里写的是 OpenTelemetry Go SDK v1.39.0+**；`go.mod` 要求
  `go.opentelemetry.io/otel` v1.46.0，而 `semconv/v1.43.0` 最早出现在 otel v1.45.0，
  所以 v1.39.0 本来就编译不过这行 import。该行现已与 `go.mod` 对齐。

## 升级说明（v1.5.0）

`InitTracer` 的签名**和原有行为**都保持不变，因此现有调用方不受影响。除了一处 nil 变成
了真实值、以及三个测试钩子不再导出之外，其余都是新增。

- **TLS 与采样现在可配置。** 它们此前是硬编码的——`otlptracehttp.WithInsecure()` 和
  `AlwaysSample()`——注释里让读者"到生产环境请改掉"，却没有任何途径可改。
  **于是每个部署都在以明文导出链路数据，并且 100% 全量记录。**
  `InitTracerWithConfig` 接收一个带 `Insecure`、`TLSConfig`、`SampleRatio`、
  `SampleNone` 和 `ExportTimeout` 的 `Config`。如果你读了旧 README 并以为自己已经配好了
  TLS 或采样比率——并没有，请迁移到 `InitTracerWithConfig`。
- **没有端点时 `InitTracer` 返回空实现 provider，而不是 `(nil, nil)`。** 惯用写法
  `tp, err := InitTracer(...)` 接 `defer tp.Shutdown(ctx)` 在链路追踪关闭时会空指针
  解引用。如果你为此加过 nil 判断，现在不需要了。
- **包级 tracer 状态已加锁。** `InitTracer` 与 `GetTracer` 或 `IsEnabled` 并发是一个
  数据竞争，因为测试是顺序执行的，所以从未被发现。
- **`SetResourceNewFunc`、`SetOtlptraceNewFunc` 和 `ResetHooks` 已从公开 API 移除。**
  它们此前位于 `test_helpers.go`——一个导入了 `testing` 的普通源文件——这会把 testing 包
  链接进每个依赖本库的生产二进制、把它的 `-test.*` 参数注册进 `flag.CommandLine`，并让
  进程里任何代码都能在运行时替换链路导出器。该文件现已更名为 `export_test.go`，只在测试
  时编译。
- **`SetupTestTracer` 接收 `tracing.TestingT`**（`Helper` + `Cleanup`）而不是
  `*testing.T`。`*testing.T` 满足该接口，因此现有调用无需改动即可编译。
- **环境要求里写的是 Go 1.26**；`go.mod` 需要 `1.27.0`。

## 环境要求

- **Go 1.27+**（`go.mod` 声明 `go 1.27.0`）
- OpenTelemetry Go SDK v1.46.0+（`go.mod` 要求 `go.opentelemetry.io/otel` v1.46.0）

## 许可证

本项目采用 Apache License 2.0 许可证 - 详见 [LICENSE](LICENSE) 文件。

## 贡献

欢迎贡献！请随时提交 Pull Request。
