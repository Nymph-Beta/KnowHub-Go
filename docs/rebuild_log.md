好的，遵照您的要求，我将保持所有文字内容不变，仅对 Markdown 格式进行优化和规范化，使其结构更清晰、更合理。

---

## 复现 and 学习 PaiSmart_Go项目

-   **目的**：学习go语言特性，与java区别，RAG相关功能的自己实现

### 阶段1 项目初始化与配置管理

-   **任务**： 搭建项目骨架，让程序能够读取配置文件

#### 1. 思考目录结构

参考Go项目的标准目录结构参考：[golang-standards/project-layout](https://github.com/golang-standards/project-layout/blob/master/README_zh.md) - Go 社区推荐的项目结构。

从中了解到 `/cmd`、`/internal`，`/pkg`，`/verndor` 目录区别：

-   `cmd` 项目的主干。
-   `internal` 私有应用程序和库代码。
-   `pkg` 外部应用程序可以使用的库代码，其他项目可以导入这些库。

#### 2. 初始化go模块

```bash
go mod init paismart-go-v2
```

#### 3. Viper配置库

Viper 是一个完整的 Go 应用程序配置解决方案，包括 12-Factor 应用。它旨在适用于任何应用程序，并能处理所有类型的配置需求和格式。可被视为满足您应用程序所有配置需求的注册表。

```bash
go get github.com/spf13/viper
```

支持的配置文件格式多样，包括json、yaml、toml等。

`viper.SetConfigFile()` 和 `viper.SetConfigName()` 用于告诉viper去哪里找配置文件，区别在于`SetConfigName`用于灵活查找，只需要指定文件名的主体，根据`AddConfigPath()` 设置的搜索路径，去寻找同名的文件，会自动尝试所有支持的扩展名；`SetConfigFile`显式指定完整的文件路径。

Viper 使用 `mapstructure` 库来实现配置到结构体的反序列化。

#### 4. 设计配置结构

参考原项目，阶段一需要的基本配置需要server下的port和mod。

具体的实现在：

-   `configs/config.yaml`（最简版本）
-   `internal/config/config.go`（配置结构体 + Init 函数）
-   `cmd/server/main.go`（调用 Init 并打印配置）

### 阶段 2: 日志系统 & HTTP 服务器

#### 1. 扩展配置文件

参考原有配置，并更新 `internal/config/config.go` 添加 `LogConfig` 结构体。

-   `format`：控制输出格式 json便于采集；console便于阅读
-   `level`：控制日志级别过滤

**config.yaml更新：**

```yaml
log:
  level: "debug"
  format: "json"
  output_path: "./logs"
  error_output_path: "./logs/error.log"
  maxsize: 100
  maxbackups: 10
  maxage: 30
  compress: true
  time_format: "2006-01-02 15:04:05.000"
```

**config.go更新：**

```go
type LogConfig struct {
	Level           string `mapstructure:"level"`
	Format          string `mapstructure:"format"`
	OutputPath      string `mapstructure:"output_path"`
	ErrorOutputPath string `mapstructure:"error_output_path"`
	Maxsize         int    `mapstructure:"maxsize"`
	Maxbackups      int    `mapstructure:"maxbackups"`
	Maxage          int    `mapstructure:"maxage"`
	Compress        bool   `mapstructure:"compress"`
	TimeFormat      string `mapstructure:"time_format"`
}
```

#### 2. Zap 日志库

截止到2026 年 1 月，一般的go预研项目会采用：`log/slog` (Go 标准库)、`uber-go/zap`、`rs/zerolog`三者其一。

-   **计划**：先使用zap来实现，后续可换用slog，不知是否可行。

`zap.Logger`：**极快**，内存分配极少。它不做任何类型推断，不使用反射；但你必须明确告诉它你传的是什么类型。

```go
// 必须用 zap.String, zap.Int 包装
logger.Info("登录成功", zap.String("user", "admin"), zap.Int("id", 123))
```

`zap.SugaredLogger`：在 `zap.Logger` 外面包裹了一层“语法糖”（Sugar）。方便，支持 `printf` 格式，支持弱类型。

```go
// 像 fmt.Printf 一样爽
sugar.Infof("登录成功 user:%s id:%d", "admin", 123)
// 或者用键值对
sugar.Infow("登录成功", "user", "admin", "id", 123)
```

`zap.NewProductionConfig()` 和 `zap.NewDevelopmentConfig()` 是 Zap 预设的两套“套餐”，分别对应**上线后**和**写代码时**的需求。

| 特性         | Production (生产环境)       | Development (开发环境)        |
| :----------- | :-------------------------- | :---------------------------- |
| **格式**     | **JSON** (机器好读)         | **Console** (人眼好读)        |
| **默认级别** | **Info** (忽略 Debug)       | **Debug** (全打印)            |
| **颜色**     | 无颜色                      | **有颜色** (Debug灰, Error红) |
| **时间格式** | Unix 时间戳 (如 167888...)  | ISO8601 (如 2023-01-01...)    |
| **堆栈跟踪** | 只有 Error/Fatal 才打印堆栈 | Warn 以上就打印堆栈           |

`log.Sync()`如果程序突然退出（无论是正常结束，还是 Panic 崩溃，还是被 `kill`），缓冲区里可能还有最后几十条日志**在内存里没来得及写到硬盘**。它把缓冲区里剩下的东西全部强制写到硬盘里。

`defer` 保证了无论函数是正常执行完，还是中间出了错崩溃了，`Sync()` **一定会**在程序退出的最后一刻被执行。

**更新：**

-   **config.yaml**：更新log相关配置
-   **config.go**：更新log相关type映射
-   新建 **log.go**：实现zap的实现，先实现日志级别设置；配置编码格式和开发环境；设置输出目录。最后构建logger，并实现wrapper包装
-   更新 **main.go**：初始化log，并新建一个test函数测试

#### 3. Gin框架，实现中间件

Gin是一个web开发框架，提供类似Martini的API，但性能更佳。它封装了底层逻辑，代码更简洁。该项目中Gin 负责接收前端请求、调用的业务逻辑、最后返回结果给前端的核心骨架。

gin本身有自己的默认架构。可以对比**`gin.Default()`**和**`gin.New()` + 自定义**。

核心区别是中间件（Middleware）。

**`gin.Default()`**适合写 Demo 或简单项目，默认的Logger格式是固定的，一般情况下需要将日志输出为 JSON 格式并存入文件（为了给 ELK、ES 等系统分析），默认的 Logger 满足不了需求。

`gin.New()` 因为该项目使用的是自定义的zap日志格式。此时的 `r` 是一张白纸。如果你直接运行，既没有日志，代码 panic（崩溃）了服务也会直接退出，需要手动添加设计中间件。

对于中间件的设计，一般来说是**前置处理（请求刚进来）**、**执行业务逻辑（`c.Next()`）**、**后置处理（响应返回时）**。如果你调用 `c.Next()`，或者在 `c.Next()` 之前就打印日志，你就**拿不到**真实的状态码和耗时。只有等 `c.Next()` 返回了，才代表后面的业务跑完了。

即：中间件A (前置代码) -> **调用 `c.Next()`** -> 暂停 A，进入 B 中间件B (前置代码) -> **调用 `c.Next()`** -> 暂停 B，进入业务 **业务逻辑执行** (返回数据) -> **回到 B** (执行 `c.Next()` 后的代码) -> **回到 A** (执行 `c.Next()` 后的代码)。

**中间件的详细设计**

1.  `bodyLogWriter` 用于捕获响应体，通过*嵌入 Gin 原生的 Writer，继承它所有功能（Status(), Header() 等）*，并且自带记录的body `bytes.Buffer`。
2.  之后重写`Write`方法，*当业务代码调用 c.JSON() 时，实际上调用的是这个 Write*，将响应写入 `gin.ResponseWriter` 和一个内部的 buffer。
3.  之后`RequestLogger`分为三步，**Read**: `ioutil.ReadAll` 会把流读空。**Restore**: `bytes.NewBuffer` 把读出来的字节变成一个新的 Reader。`ioutil.NopCloser` 把它包装成符合 `ReadCloser` 接口的对象。**Assign**: 赋值回 `c.Request.Body`，这样后续的业务逻辑完全感觉不到 Body 已经被读过一次了。
4.  使用自定义的 `ResponseWriter` 捕获响应，之后所有 `c.JSON/c.String` 的输出都会经过 `blw.Write`。

> **隐患**：代码里 `ioutil.ReadAll(c.Request.Body)` 会把整个 Body 读入内存。如果用户上传了一个 **500MB 的文件**，这个中间件会瞬间消耗 500MB 内存来存日志。（之后解决？）
>
> **你的担忧是对的！ 这在阶段二是 OK 的，但在阶段 7（分片上传）需要特殊处理**

**更新：**

-   创建中间件`logging.go`
-   **main.go**: 更新gin相关配置，启动

#### 4. 优雅停机

开始使用的是`r.Run(":" + cfg.Server.Port)`。它内部也是调用 `http.ListenAndServe`，但无法控制关闭流程，因为它不返回 `http.Server` 对象。

所以需要**手动创建** `http.Server` 对象，即申请分配一个 `http.Server` 结构体的内存在 `Addr` 和 `Handler` 这两个格子里写上数据。

```go
srv := &http.Server{
    Addr:    fmt.Sprintf(":%s", cfg.Server.Port),
    Handler: r, // 把 Gin 的引擎传进去
}
```

之后使用 `if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed `，并且使用`go func`，因为 `srv.ListenAndServe()` 是一个死循环，如果不把它丢到子协程（goroutine）里，代码就会卡死在这一行，永远走不到下面的“监听停机信号”的代码。

`err != http.ErrServerClosed` 当我们稍后主动调用 `srv.Shutdown()` 关闭服务时，`ListenAndServe` 会返回一个特定的错误 `http.ErrServerClosed`。这是正常的关闭流程，不是故障。

之后创建通道，用来接收系统信号。通过`signal.Notify `通知如果收到 SIGINT (Ctrl+C) 或 SIGTERM (Docker/K8s 停止容器)， 不要直接kill，而是把信号发到 quit 通道里。
**SIGINT (Signal Interrupt)**通常是终端按下了 **`Ctrl + C`**。**SIGTERM (Signal Terminate)**通常是 **Docker、Kubernetes** 或 `kill` 命令（默认不带参数时）。

**更新：**

- 在main.go中实现优雅停机

### 阶段3：数据库连接 & 用户模型 

#### 1. 启动MySql服务

原项目使用Docker compose启动MySQL，配置在 `deployments/docker-compose.yaml`

该文件定义了Paismart项目中所有基础设施服务，通过 Docker Compose 一键启动整个环境

- `docker-compose.yaml`中的参数：

| 参数                | 值           | 含义                              |
| :------------------ | :----------- | :-------------------------------- |
| container_name      | mysql        | 容器名称，方便识别和管理          |
| image               | mysql:8      | 使用 MySQL 8.x 官方镜像           |
| restart             | always       | 容器退出后自动重启（除非手动停止) |
| MYSQL_ROOT_PASSWORD | PaiSmart2025 | 设置 root 用户密码                |
| MYSQL_DATABASE      | PaiSmart     | 容器启动时自动创建的数据库名      |

| 部分 | 含义                         |
| :--- | :--------------------------- |
| 3307 | 宿主机端口（你的电脑）       |
| 3306 | 容器内端口（MySQL 默认端口） |

- 检查MySql是否启动

```cmd
docker ps | grep mysql
# 或者查看 docker-compose 状态
cd ~/Projects/paismart-go/deployments && docker compose ps
```

- 检查数据库名和密码

```cmd
docker inspect mysql --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -i mysql
```

------

#### 2: 扩展配置文件

任务：在 config.yaml 中添加数据库配置

思考：

- 原项目的数据库配置在 configs/config.yaml 的哪里？

- DSN (Data Source Name) 的格式是什么？

- 为什么 DSN 中有 parseTime=True？

```yaml
# configs/config.yaml - 需要添加什么？
database:
 mysql:
  dsn: "用户名:密码@协议(主机:端口)/数据库名?参数" # 参考原项目格式
```
MySQL 驱动默认把 DATE/DATETIME/TIMESTAMP 当 []byte 或 string 返回，不会自动转成 Go 的 time.Time。加上 parseTime=True 后，驱动会把这些类型解析成 time.Time 。
代码里可以直接用 time.Time 做比较、格式化、计算。GORM 等 ORM 的 time.Time 字段能正确映射到数据库的日期时间类型

然后：更新 internal/config/config.go 添加 DatabaseConfig 结构体

#### 3. GORM框架
GORM 是 Go 语言中最流行的 ORM（Object-Relational Mapping，对象关系映射）框架。它的核心作用是让你用 Go 的结构体来操作数据库，而不用写大量的原生 SQL。

区别：
- 没有 ORM：需要手写 SQL（SELECT * FROM users WHERE id = ?），然后手动把查询结果映射到 Go 结构体
- 有 GORM：你直接操作 Go 结构体，GORM 自动生成 SQL 并完成映射
```go
// 没有 GORM - 手写 SQL + 手动扫描
rows, _ := db.Query("SELECT id, name, email FROM users WHERE id = ?", 1)
var user User
rows.Scan(&user.ID, &user.Name, &user.Email)

// 有 GORM - 一行搞定
db.First(&user, 1)  // 自动生成 SELECT * FROM users WHERE id = 1
```

之后可以自定义用户模型和执行CRUD操作

并且可以和Gin结合：
```go
r.GET("/users/:id", func(c *gin.Context) {
    var user User
    if err := db.First(&user, c.Param("id")).Error; err != nil {
        c.JSON(404, gin.H{"error": "用户不找到"})
        return
    }
    c.JSON(200, user)
})
```

更新：\cmd\server\gorm_demo

##### 注意：
gorm中的日志记录是内置的日志系统，默认输出到标准输出（控制台）。但在该项目中日志记录是封装zap:`import "pai_smart_go_v2/pkg/log"`

如果想让 GORM 使用 zap，我使用的方法是使用 `moul.io/zapgorm2` 第三方库，可以将 GORM 的日志桥接到 zap 。在`log.go`中更新：通过 `GetLogger()` 导出原始的` *zap.Logger`
```go
zapLogger = logger // 新增：导出原始 logger
sugarLogger = logger.Sugar()

func GetLogger() *zap.Logger {
	return zapLogger
}
```
#### 4. 设计User模型、

参考数据库中数据类型来写

更新：`user.go`

具体细节：
- `gorm:"primaryKey;autoIncrement` : 声明主键以及主键自增。即插入时不写 ID，数据库会自动分配下一个整数。
- `json:"-"`：序列化成 JSON 时忽略该字段，即使用 json.Marshal(user) 或框架自动把 User 转 JSON，也不会把 Password 返回给前端，避免密码泄露。
- `gorm:"autoCreateTime"` 和 ` gorm:"autoUpdateTime" `：创建时自动设为当前时间。插入新记录时，不用手动填 CreatedAt，GORM 会帮你写。每次更新时自动刷新为当前时间。只要执行 Update/Save，UpdatedAt 就会更新。
- `TableName() `：GORM 默认用结构体名的复数、蛇形来猜表名：`User → users`。实现 `TableName()` 可以显式指定表名，例如固定为 `"users"`。
-  Role 字段为什么用 enum？ :`gorm:"type:enum('USER', 'ADMIN');default:'USER'" `表示在数据库里该列是 MySQL ENUM 类型，只允许 `USER` 或 `ADMIN`，默认 `USER`。

| 设计点                     | 作用                                     |
| :------------------------- | :--------------------------------------- |
| 主键 + 自增                | 唯一标识每条用户，且不用手写 ID。        |
| 唯一 + 非空（如 Username） | 登录名不重复、必填。                     |
| 密码 json:"-"              | 不在任何 JSON 里输出，保护密码。         |
| Role 用 enum               | 角色只在有限集合内，数据库和代码都一致。 |
| CreatedAt / UpdatedAt      | 自动记录创建与修改时间，便于排查和审计。 |
| TableName()                | 明确表名为 users，不依赖 GORM 默认规则。 |

整体上，这张表做到了：有唯一主键、有创建/修改时间、敏感字段不出网、角色有约束，是常见的用户表设计方式。

#### 5. 实现数据库连接模块

更新：创建 `pkg/database/mysql.go`
- 使用全局变量DB： 为了“全进程共用一个 DB 实例、各处直接用”，属于单例式的设计。
- 连接失败时调用 `log.Fatal` : 打日志并调用 os.Exit(1)，进程直接退出。后续可更改设计，使其更灵活
- 连接池： 程序启动后预先建好一批到 MySQL 的 TCP 连接，放在池子里；每次要执行 SQL 时从池里取一条连接用，用完后还回池里，而不是每次请求都新建、用完就关。

设计思路：
- 启动时初始化、全进程单例
InitMySQL(dsn) 在 main 里调用一次，成功后把 *gorm.DB 存到包级变量 DB，后续所有业务通过 database.DB 访问，不再重复建连。
- 把「能否正常服务」和「连库是否成功」绑定
连不上就 log.Fatal 退出，保证进程里只要存在，就认为 DB 已经可用，不做「DB 未初始化仍继续跑」的复杂分支。
- 显式配置连接池，面向常驻进程
用 DB.DB() 拿到底层 *sql.DB，设置 MaxIdleConns、MaxOpenConns、ConnMaxLifetime，适合多请求并发的长驻服务，而不是一次性脚本。
- 包职责单一
pkg/database 只负责「建连 + 配置连接池 + 暴露 DB」，不关心业务和路由，方便在 main 里先 config → log → database 再起 HTTP。
整体上：用全局 DB 单例 + 启动时一次初始化 + 失败即 Fatal + 连接池配置，是小型/中型 Go 服务里很常见的一种简单、清晰的数据库接入方式。

**注意：**

对于是否采用之前实现的zapLogger + GetLogger()来记录
- 仅就「初始化这段代码」本身，这里几乎不会执行业务 SQL，所以这段初始化代码本身不依赖 zapLogger + GetLogger() 也能把该记的记全。只为“初始化 MySQL 这几行”的话，不加 zapLogger + GetLogger() 也够用。
- 从「整条链路日志都进 zap」的角度，若希望“所有 GORM SQL 都进 zap”，就需要在初始化时加上 zapLogger + GetLogger()（在 gorm.Open 时传入 zapgorm2）。
**这里需要之后思考**

#### 6. 实现 AutoMigrate
该环节与原项目实现不同：
1. cmd/server/main.go 里没有 AutoMigrate
    在 `main.go` 里只有：`database.InitMySQL(cfg.Database.MySQL.DSN)`
    没有任何 `database.DB.AutoMigrate(...)` 或其它迁移调用。
    `pkg/database/mysql.go` 里的 `InitMySQL` 也只做连接和连接池配置，没有执行迁移。

2. 项目里的“自动建表”是在哪里做的
    项目没有用 GORM 自动建表，而是用 MySQL 容器首次启动时执行 DDL：
    在 `deployments/docker-compose.yaml` 里有：`docker-compose.yaml`
    MySQL 官方镜像会在第一次启动时执行 /docker-entrypoint-initdb.d/ 下的 .sql 文件。

  因此：
  自动建表 = 把 docs/ddl.sql 挂到 docker-entrypoint-initdb.d/001-ddl.sql，由 MySQL 在容器首次初始化时执行，创建 users、organization_tags、file_upload、chunk_info、document_vectors 等表。

  也就是说：“自动建表”是在 Docker 部署里通过 MySQL 的 init 脚本（docs/ddl.sql）实现的，不是在 Go 代码里用 AutoMigrate 实现的。

实现思路：
1. 学习 GORM 完整能力
    AutoMigrate 是 GORM 的核心功能之一，很多项目（特别是小型项目）都用它
2. 开发更方便
    改 struct → 重启程序 → 表结构自动更新，不用每次改 SQL 文件
3. 后续可以切换
    理解原理后，生产部署时可以改回 DDL 方式，两种方式不冲突

**原项目方式**：通过 `docs/ddl.sql` + Docker 挂载实现
- 优点：生产环境更安全，完全控制表结构
- 缺点：需要手动维护 SQL 和 Model 一致性

**本项目选择**：开发阶段使用 `db.AutoMigrate()`
- 优点：学习 GORM 能力，开发迭代方便
- 注意：生产部署时可考虑切换为 DDL 文件方式

具体实现：
- 在 `pkg/database/mysql.go` 末尾添加`RunMigrations()`,
```go
// RunMigrations 执行数据库表结构迁移（开发阶段使用）
// 使用 GORM AutoMigrate 根据 model 自动创建/更新表结构
func RunMigrations() error {
	log.Info("开始执行数据库表结构迁移...")

	// 按照依赖顺序迁移表
	if err := DB.AutoMigrate(
		&model.User{},
		// 后续阶段会继续添加：
		// &model.OrgTag{},          // 阶段 5
		// &model.Upload{},          // 阶段 6-7
		// &model.ChunkInfo{},       // 阶段 7
		// &model.DocumentVector{},  // 阶段 10
	); err != nil {
		log.Errorf("AutoMigrate 失败: %v", err)
		return err
	}

	log.Info("数据库表结构迁移完成")
	return nil
}
```
- 在 `cmd/server/main.go` 的 `database.InitMySQL(...)` 后面添加：`database.RunMigrations()`
- 新建数据库为PaiSmart_v2
- 在开发最后切换为 Docker 挂载方式

### 阶段四 用户认证（JWT）
#### 1. 理解分层架构

原项目中的三层架构设计：
- Handler层： 只负责HTTP协议。负责解析请求体、调用Service、格式化响应
- Service层： 业务逻辑编排。负责协调多个Repository、实现业务规则（密码验证、Token生成）、事务管理
- Respository层： 只负责数据库操作。负责执行SQL查询、ORM映射、返回数据模型


如果不使用三层架构，直接在Handler里写SQL，会导致：
- 🔴 无法复用：如果另一个Handler也需要登录逻辑，必须复制粘贴
- 🔴 难以测试：要测试登录逻辑，必须启动整个HTTP服务器
- 🔴 紧耦合：修改数据库查询方式，必须改所有Handler
- 🔴 可读性差：一个函数包含HTTP、业务、数据库逻辑，超过100行

举例说明：
- 代码复用： `FindByUsername`函数被多个地方调用，如果在Handler中写SQL，会多次复制
- 业务逻辑服用： 比如用户注册逻辑，该流程设计多个Repository和数据库操作，若不使用三层架构，逻辑很臃肿
- 单元测试： 分层可以独立测试每一层
- 技术栈迁移： 假设未来要把数据库从MySQL换成PostgreSQL：分层架构只需修改 Repository 实现 ，Handler 和 Service 无需改动

为什么 Repository 要定义接口（interface）？
- 为了以来倒置和可测试性。
- Service依赖的是接口，而不是具体实现，比如用户注册，可以传入任何实现`UserRespository`的对象
- 可以轻松切换实现（MySql或postgreSQL等）
- 单元测试不需要数据库

```go
Repository → Service → Handler
     ↓          ↓         ↓
   数据访问   业务逻辑   HTTP处理
```

####  2. 扩展配置文件
更新： config.yaml
```yaml
jwt:
  secret: "JWT 签名密钥, 一个 Base64 编码的 32 字节（256 位）随机数据，通常通过 openssl rand -base64 32 之类的命令生成。"
  access_token_expire_hours: Access Token 过期时间, 用户每次请求 API 时携带的短期凭证，用于身份验证。
  refresh_token_expire_days: Refresh Token 过期时间, 用于在 Access Token 过期后，免登录地获取新的 Access Token。它存储在客户端（通常是 HttpOnly Cookie 或安全存储中），生命周期更长。
```

#### 3. 实现密码加密模块

更新：新增 `pkg/hash/bcrypy.go`
参考资料：`https://pkg.go.dev/golang.org/x/crypto/bcrypt`

使用两个核心函数：
```go
func HashPassword(password string) (string, error) {
  hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}
func CheckPasswordHash(password, hash string) bool {
  return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
```

- bcrypt.DefaultCost 是 golang.org/x/crypto/bcrypt 包中定义的常量，值为 10。它代表 bcrypt 算法的工作因子（work factor），即内部哈希迭代次数的指数：实际迭代次数为 2^cost 次
- cost 用于对抗硬件提升、可调节时间成本、并且可以向前兼容。
- 相比于MD5/SHA256，bcrypt专为密码存储设计。通过cost调节速度，自动生成并嵌入结果中，并且bcrypt 内存访问模式对 GPU 不友好

CheckPasswordHash 为什么返回 bool 而不是 error？
底层的 `bcrypt.CompareHashAndPassword` 实际上是返回 `error` 的——密码不匹配时返回 bcrypt.ErrMismatchedHashAndPassword，格式错误时也会返回相应的 `error`。

这里封装成 bool 是一个有意的简化设计，原因是：
- 调用方只关心"对不对"：在登录场景中，不论是密码错误还是哈希格式损坏，对用户的响应都是一样的——"认证失败"。区分具体错误类型对业务逻辑没有意义，反而会增加调用方的代码复杂度。
- 安全考量：向外暴露具体的错误类型（如"哈希格式无效" vs "密码不匹配"）可能给攻击者提供信息泄露的机会（信息枚举攻击）。
- API 简洁性：`if !CheckPasswordHash(pwd, hash)` 比 `if err := CheckPasswordHash(pwd, hash); err != nil` 更直观。


#### 4. 实现 JWT 模块

新建： `pkg/token/jwt.go` `pkg/token/jwt_test.go`

JWT（JSON Web Token）是一种开放标准（RFC 7519），用于在各方之间以 JSON 对象 的形式安全传输信息。它通过数字签名（HMAC、RSA 或 ECDSA）来保证信息可验证、可信任。

使用场景：
- 授权（Authorization）：用户登录后，后续每个请求携带 JWT，服务端据此判断是否允许访问资源。
- 信息交换：签名确保发送方身份和内容未被篡改。

主要结构是三段式：

一个 JWT ：xxxxx.yyyyy.zzzzz，由 . 分隔为三部分：

| 部分      | 内容           | 说明                                                    |
| :-------- | :------------- | :------------------------------------------------------ |
| Header    | 算法 + 类型    | 如 {"alg": "HS256", "typ": "JWT"}                       |
| Payload   | Claims（声明） | 用户信息、过期时间等，如 {"sub": "123", "name": "John"} |
| Signature | 签名           | 用 Header + Payload + 密钥 生成，防篡改                 |

每部分都经过 Base64Url 编码。

主要工作流程：
1. 客户端用账号密码登录
2. 服务端验证后返回一个 JWT
3. 客户端之后每次请求在 Authorization: Bearer <token> 头中携带该 JWT
4. 服务端验证 JWT 有效后，允许访问受保护资源

代码设计思路：
```
密钥 + 过期时间 → 封装进一个结构体 → 暴露 Generate / Verify 方法
```

`pkg/token/jwt.go` 基于 `golang-jwt/jwt/v5` 实现，核心结构：

- **JWTManager**：封装密钥和过期时间，暴露 `GenerateToken` / `VerifyToken` 两个方法
- **CustomClaims**：嵌入 `jwt.RegisteredClaims`（标准字段 iss/exp/iat/nbf），扩展 UserID、Username、Role、TokenType
- **双 Token 机制**：`GenerateToken` 同时返回 access token（短期）和 refresh token（长期）
- **签名算法**：使用 HMAC-SHA256（对称密钥，签名和验证用同一个 secret）

改动记录：

1. **添加 TokenType 字段**：CustomClaims 增加 `token_type` 字段（值为 `"access"` 或 `"refresh"`），防止攻击者拿 refresh token 冒充 access token 调用 API。业务层中间件通过检查 `claims.TokenType` 来区分。
2. **WithValidMethods 替代手动类型断言**：VerifyToken 中原先通过 `token.Method.(*jwt.SigningMethodHMAC)` 手动检查算法类型（允许 HS256/384/512 三种），改为使用 `jwt.WithValidMethods([]string{"HS256"})` 精确限制只允许 HS256，更符合 v5 API 风格，防止算法篡改攻击。


#### 5. 实现 Repository 层
具体实现： `internal/repository/user_repository.go` `internal/repository/user_repository_test.go`

设计思路： 接口 + 结构体实现
1. 接口和实现分离
  先定义 UserRepository 接口。
  ```go
  type UserRepository interface {
   // 定义的用户数据的各个持久化操作
  }
  ```
  再定义userRepository ： UserRepository 接口的 GORM 实现。
  ```go
  type userRepository struct {
    db *gorm.DB
  }
  ```
  - UserRepository（大写 U）：公开的接口，定义"能做什么"
  - userRepository（小写 u）：私有的结构体，定义"怎么做"
  外部包只能看到接口，看不到具体实现。这就是 Go 的封装方式。

2. 工厂函数返回接口类型
  ```go
  // NewUserRepository 创建一个新的 UserRepository 实例。
  func NewUserRepository(db *gorm.DB) UserRepository {
    return &userRepository{db: db}
  }
  ```
  NewUserRepository 接受一个 *gorm.DB 参数，返回的是接口类型 UserRepository，而不是具体的 *userRepository。调用者永远不知道底层实现是什么。

  优势：
  1. 解耦 —— Service 层不依赖具体的数据库实现
    - Service 只知道"有一个 UserRepository 接口，它能 Create、FindByUsername 等"，但完全不知道底层用的是 GORM、还是原生 SQL、还是 MongoDB。这就是依赖倒置原则 (DIP)。
  2. 方便测试
    - 因为 Service 依赖的是接口，在写单元测试时，可以创建一个 Mock 实现，不需要连接真实数据库
  3. 可替换性 —— 换数据库只改 Repository 层
---
#### 6. 实现 Service 层

更新内容： `internal/service/user_service.go`  `internal/service/user_service_test.go`

先完成用户相关 Service ，不含组织标签的逻辑

实现：
```go
type UserService interface {
    Register(username, password string) (*model.User, error)
    Login(username, password string) (accessToken string, err error)
    GetProfile(username string) (*model.User, error)
}
```

Service 层处于中间位置，向上为 Handler 提供简洁的业务接口（一个方法调用完成整个业务），向下通过组合多个 Repository 和工具包来实现复杂的业务流程。

登录失败应该返回"用户不存在"还是"凭证无效"？
- 应该统一返回"凭证无效"（即 ErrInvalidCredentials），这是正确的做法。否则可能会被枚举攻击。

Service 层应该调用 log 记录日志吗？
- 对于目前的项目阶段，当前在 Service 里直接用 log.Errorf 是可以的，先把功能做通。等后续项目复杂了，再考虑将 logger 改为依赖注入的方式。

#### 7. 实现 Handler 层

更新： `internal/handler/user_handler.go` `internal/handler/user_handler_test.go`

```go
type UserHandler struct {
    userService service.UserService
}

// 请求体结构
type RegisterRequest struct {
    Username string `json:"username" binding:"required"`
    Password string `json:"password" binding:"required"`
}

// Handler 方法
func (h *UserHandler) Register(c *gin.Context) { ... }
func (h *UserHandler) Login(c *gin.Context) { ... }
func (h *UserHandler) GetProfile(c *gin.Context) { ... }
```

##### 知识点

**`binding:"required"` 与 Gin 参数验证**

- 结构体标签可以有多个，用空格隔开：`json:"username"` 负责 JSON 序列化，`binding:"required"` 负责验证。
- Gin 底层使用 [go-playground/validator](https://github.com/go-playground/validator) 库。
- `binding:"required"` 表示字段必须存在且不能为零值（空字符串、0、nil）。
- 常用验证规则示例：
  - `binding:"required,min=3,max=32"` — 必填，3-32 字符
  - `binding:"required,email"` — 必须是邮箱格式
  - `binding:"oneof=USER ADMIN"` — 枚举值限制
  - `binding:"omitempty,gte=0,lte=150"` — 可选，但传了就必须在范围内

**`c.ShouldBindJSON()` vs `c.BindJSON()`**

- `ShouldBindJSON`：验证失败只返回 error，不自动写响应，由开发者自己控制返回格式。**推荐使用。**
- `BindJSON`：验证失败会自动调用 `c.AbortWithError(400, err)` 写一个不可定制的响应，失去控制权。
- 结论：几乎所有情况都用 `Should` 前缀的方法（`ShouldBindJSON`、`ShouldBindQuery`、`ShouldBindUri`）。

**HTTP 状态码选择**

| 状态码 | 含义 | 本项目场景 |
|-------|------|----------|
| 200 OK | 请求成功 | Login 成功、GetProfile 成功 |
| 201 Created | 成功创建新资源 | Register 成功 |
| 400 Bad Request | 请求格式错误 | JSON 解析失败、缺少必填字段 |
| 401 Unauthorized | 身份验证失败 | 登录密码错误、Token 无效/过期 |
| 403 Forbidden | 有身份但没权限 | 普通用户访问管理员接口 |
| 404 Not Found | 资源不存在 | GetProfile 查无此用户 |
| 409 Conflict | 资源冲突 | 注册时用户名已存在 |
| 500 Internal Server Error | 服务器内部出错 | 数据库挂了、Token 生成失败 |

- 401 vs 403：401 是"不知道你是谁"（未认证），403 是"知道你是谁但没权限"（已认证但未授权）。

**Handler 层错误处理要点**

- 用 `mapServiceError()` 将 Service 哨兵错误映射到正确的 HTTP 状态码，避免所有错误都返回 500。
- 错误分支必须 `return`，否则会继续执行成功响应，导致重复写响应。
- 对外响应不包含 `err.Error()`，避免泄露数据库/内部实现细节。
- `c.Get()` 返回 `any` 类型，使用前需要类型断言：`user, ok := userVal.(*model.User)`。

#### 8. 实现 Auth 中间件
更新：`internal/middleware/auth.go`

##### 整体流程

```
请求进来 → [AuthMiddleware] → 通过 → Handler 处理业务
                │
                ├─ 0. 依赖检查（jwtManager/userService 是否为 nil）
                ├─ 1. 提取 Bearer Token
                ├─ 2. 验证 Token 签名和有效期
                ├─ 3. 检查 Token 类型（必须是 access）
                ├─ 4. 查数据库确认用户存在
                ├─ 5. 注入 claims 和 user 到上下文
                └─ 6. c.Next() 放行
                │
                └─→ 任一步失败 → AbortWithStatusJSON → 请求到此结束
```

##### 第零步：防御性检查

```go
if jwtManager == nil || userService == nil {
    c.AbortWithStatusJSON(http.StatusInternalServerError, ...)
    return
}
```
确保依赖已正确注入，防止后续代码 nil panic。

##### 第一步：提取 Bearer Token

```go
tokenString, err := extractBearerToken(c.GetHeader("Authorization"))
```
从 HTTP 请求头 `Authorization: Bearer xxxxx` 中提取出 Token 字符串。Bearer 是 OAuth 2.0 / JWT 的行业标准格式。

`extractBearerToken` 的实现改进：
- 用 `strings.Fields` 替代 `HasPrefix + TrimPrefix`，自动处理多余空格
- 用 `strings.EqualFold` 做大小写不敏感比较，兼容 `bearer`、`BEARER` 等写法

##### 第二步：验证 Token

```go
claims, err := jwtManager.VerifyToken(tokenString)
if err != nil || claims == nil {
    c.AbortWithStatusJSON(http.StatusUnauthorized, ...)
    return
}
```
调用 `jwtManager.VerifyToken` 检查 Token 签名是否正确、是否过期。验证失败就 Abort（中止），请求到此结束，不会到达 Handler。

##### 第三步：检查 Token 类型

```go
if claims.TokenType != token.TokenTypeAccess {
    c.AbortWithStatusJSON(http.StatusUnauthorized, ...)
    return
}
```
受保护接口只接受 access token，拒绝 refresh token。防止攻击者拿 refresh token 冒充 access token 来访问 API（这就是为什么 `jwt.go` 中要区分 `TokenTypeAccess` 和 `TokenTypeRefresh`）。

##### 第四步：查数据库确认用户存在

```go
user, err := userService.GetProfile(claims.Username)
if err != nil {
    switch {
    case errors.Is(err, service.ErrUserNotFound):
        // 用户已删除 → 401
    default:
        // 数据库错误 → 500（不泄露细节）
    }
    return
}
```
即使 Token 有效，用户也可能已被删除或禁用，所以需要查一次数据库确认。错误处理使用 Service 层的哨兵错误做精确匹配。

##### 第五步：注入上下文 + 放行

```go
c.Set("claims", claims)  // 后续 Handler 通过 c.Get("claims") 获取 JWT Claims
c.Set("user", user)      // 后续 Handler 通过 c.Get("user") 获取 *model.User
c.Next()                  // 放行，继续执行下一个中间件或 Handler
```
注意：`c.Get("user")` 返回的是 `any` 类型，Handler 中使用时需要类型断言 `user, ok := userVal.(*model.User)`。

##### 学到的知识点

**Gin 中间件的核心 API：**
- `c.Abort()` — 中止请求链，后续的中间件和 Handler 都不会执行
- `c.AbortWithStatusJSON()` — 中止 + 写 JSON 响应，一步到位
- `c.Next()` — 放行，继续执行下一个中间件或 Handler
- `c.Set(key, value)` / `c.Get(key)` — 在中间件和 Handler 之间传递数据

**`c.AbortWithStatusJSON()` vs `c.JSON()` 的区别：**

核心区别是**是否中止请求链**：
- `c.JSON()` — 只写 JSON 响应，不影响代码流程，后续中间件和 Handler 照常执行
- `c.AbortWithStatusJSON()` — 写 JSON 响应 + 中止请求链，后续中间件和 Handler 不再执行

```
请求 → 中间件A → 中间件B → Handler

中间件A 调 c.JSON() + return       → 中间件B 会执行，Handler 会执行
中间件A 调 c.AbortWithStatusJSON() → 中间件B 不执行，Handler 不执行
```

使用场景：
- **中间件里** — 用 `c.AbortWithStatusJSON()`，阻止请求继续往下走到 Handler
- **Handler 里** — 用 `c.JSON()` + `return`，Handler 是请求链最后一环，不需要 Abort

注意：`c.Abort()` 不会终止当前函数的执行，它只是设置了一个标志位。所以 Abort 后仍需手动 `return`，否则当前函数后面的代码还会跑。

**为什么 Token 要区分 access 和 refresh 类型？**
- access token：短期有效（如 15 分钟），用于访问 API
- refresh token：长期有效（如 24 小时），仅用于换取新的 access token
- 如果不区分，攻击者拿到 refresh token 就能直接访问 API，refresh token 的长有效期会放大风险

#### 9. 注册路由

更新：`cmd/server/main.go`

##### main 函数整体流程

```
1. 加载配置 config.Init()
2. 初始化日志 log.Init()
3. 连接数据库 database.InitMySQL() + RunMigrate()
4. 依赖注入 Repository → JWTManager → Service → Handler
5. 创建 Gin 引擎 + 全局中间件
6. 注册路由
7. 启动 HTTP 服务器 + 优雅停机
```

##### 依赖注入链

依赖注入顺序体现了各层之间的依赖关系，**从底层到上层**：

```go
// 1. Repository（最底层，依赖数据库连接）
userRepo := repository.NewUserRepository(database.DB)

// 2. JWT Manager（独立组件，从配置读取密钥和过期时间）
jwtManager := token.NewJWTManager(
    cfg.JWT.Secret,
    time.Duration(cfg.JWT.AccessTokenExpireHours) * time.Hour,
    time.Duration(cfg.JWT.RefreshTokenExpireDays) * 24 * time.Hour,
)

// 3. Service（业务层，注入 Repository 和 JWTManager）
userService := service.NewUserService(userRepo, jwtManager)

// 4. Handler（接口层，注入 Service）
userHandler := handler.NewUserHandler(userService)
```

注入链路图：
```
database.DB → UserRepository → UserService → UserHandler
                                    ↑
cfg.JWT     → JWTManager ──────────┘
```

##### 创建 Gin 引擎与全局中间件

```go
gin.SetMode(cfg.Server.Mode)               // 设置运行模式（debug/release）
r := gin.New()                              // 创建空引擎（不带默认中间件）
r.Use(middleware.RequestLogger(), gin.Recovery())  // 注册全局中间件
```

- `gin.New()` vs `gin.Default()`：`Default()` 自带 Logger 和 Recovery，`New()` 是空的，我们用自定义的 `RequestLogger` 替代默认 Logger
- `gin.Recovery()` — 捕获 panic，防止服务崩溃，返回 500

##### 路由注册

```go
// 用户相关路由，统一前缀 /api/v1/users
users := r.Group("/api/v1/users")
{
    // 公开路由（无需认证）
    users.POST("/register", userHandler.Register)   // POST /api/v1/users/register
    users.POST("/login", userHandler.Login)          // POST /api/v1/users/login

    // 受保护路由（需要认证）
    authed := users.Group("/")
    authed.Use(middleware.AuthMiddleware(jwtManager, userService))
    {
        authed.GET("/me", userHandler.GetProfile)    // GET /api/v1/users/me
    }
}
```

路由结构：
```
/api/v1/users
├── POST /register          ← 公开，无需 Token
├── POST /login             ← 公开，无需 Token
└── [AuthMiddleware]        ← 需要 Bearer Token
    └── GET /me             ← 受保护，返回当前用户信息
```

- `r.Group()` — 创建路由组，共享前缀和中间件
- 只有 `authed` 组使用了 `AuthMiddleware`，所以 `/register` 和 `/login` 不需要 Token
- 注意：**路由注册必须在 `r := gin.New()` 之后**，否则 `r` 未定义会编译报错

##### 知识点

**Gin 路由组（Group）的作用：**
- 共享 URL 前缀：`r.Group("/api/v1/users")` 下的所有路由自动带 `/api/v1/users` 前缀
- 共享中间件：`authed.Use(AuthMiddleware)` 只对 `authed` 组内的路由生效，不影响外面的公开路由
- 可以嵌套：在 `users` 组内再创建 `authed` 子组，实现"部分路由需要认证"的效果

**`gin.New()` vs `gin.Default()`：**
- `gin.Default()` = `gin.New()` + Logger 中间件 + Recovery 中间件
- 当你有自定义日志中间件（如 `RequestLogger`）时，用 `gin.New()` 避免重复日志

### 阶段五 Redis 集成 & 组织标签
#### 1. 启动 Redis 并扩展配置
该环节与原项目实现不同：

1. 原项目的 Redis 是通过 `deployments/docker-compose.yaml` 启动的 Docker 容器

    ```yaml
    redis:
      image: redis:7.2-alpine
      ports:
        - "6380:6379"
      command: redis-server --bind 0.0.0.0 --port 6379 --requirepass PaiSmart2025 --appendonly yes
    ```

    docker-compose 中配置了以下参数：

    | 参数 | 值 | 含义 |
    | :--- | :--- | :--- |
    | `--bind 0.0.0.0` | 监听所有网络接口 | 允许容器外部连接 |
    | `--port 6379` | 容器内部端口 | Redis 的默认端口就是 **6379** |
    | `--requirepass PaiSmart2025` | 访问密码 | 客户端连接时必须提供 |
    | `--appendonly yes` | 开启 AOF 持久化 | 每次写操作追加到日志文件，防止数据丢失 |
    | `ports: "6380:6379"` | 端口映射 | 宿主机 6380 → 容器内 6379 |

    然后在 `configs/config.yaml` 中手动填写对应的连接信息：
    ```yaml
    redis:
      addr: "127.0.0.1:6380"  # 对应 docker-compose 映射的宿主机端口
      password: "PaiSmart2025"  # 对应 --requirepass
      db: 0
    ```

    也就是说：Docker 层和应用层各自写了一份配置，靠开发者手动保证一致。

2. `redis.Options` 中 `DB` 参数的含义

    Redis 内置了 **16 个逻辑数据库**（编号 0-15），共享同一个 Redis 实例的内存，但数据互相隔离。`DB` 参数指定连接哪个逻辑数据库：
    - `DB: 0` — 第 0 号数据库（默认值）
    - `DB: 1` — 第 1 号数据库
    - ... 以此类推到 `DB: 15`

    典型用途：在同一个 Redis 实例上隔离不同业务的数据（如 DB 0 给缓存，DB 1 给 session）。

**原项目方式**：Docker 容器启动 Redis + docker-compose 中配置参数
- 优点：团队协作，一键 `docker compose up` 启动所有服务
- 缺点：docker-compose.yaml 和 config.yaml 两处配置需手动保持一致

**本项目选择**：使用 WSL 本地已安装的 Redis（systemd 服务），不依赖 Docker
- 优点：开发轻量，用到什么启动什么；只维护 config.yaml 一份配置
- 注意：部署时再统一整理到 docker-compose.yaml

当前环境：
```
WSL 本地 Redis: 127.0.0.1:6379, 无密码, systemd 管理 (redis-server.service)
Docker Redis:   127.0.0.1:6380, 密码 PaiSmart2025 (原项目在用，互不干扰)
```

v2 项目使用 DB 1 而非默认的 DB 0，避免与终端手动测试或其他工具的数据互相污染。

具体实现：

- 更新 `configs/config.yaml`，在 `database` 下添加 `redis` 配置：
```yaml
database:
  mysql:
    dsn: "root:PaiSmart2025@tcp(127.0.0.1:3307)/paismart_v2?parseTime=True"
  redis:
    addr: "127.0.0.1:6379"
    password: ""
    db: 1
```

- 更新 `internal/config/config.go`，在 `DatabaseConfig` 中添加 `RedisConfig`：

- 新建 `pkg/database/redis.go`，参考原项目实现 `InitRedis`：

- 更新 `cmd/server/main.go`，在 `database.InitMySQL(...)` 后添加：
```go
database.InitRedis(cfg.Database.Redis.Addr, cfg.Database.Redis.Password, cfg.Database.Redis.DB)
```

##### 知识点

**go-redis 库**

`go-redis/redis/v8` 是 Go 社区最流行的 Redis 客户端库。核心用法：
- `redis.NewClient(&redis.Options{...})` — 创建客户端实例
- `RDB.Ping(ctx)` — 测试连接是否正常
- `Password` 为空字符串时自动跳过认证，兼容无密码的 Redis

**与 MySQL 初始化的设计对比**

| | InitMySQL | InitRedis |
| :--- | :--- | :--- |
| 全局变量 | `DB *gorm.DB` | `RDB *redis.Client` |
| 连接测试 | GORM 内部自动测试 | 手动 `Ping(ctx)` |
| 连接池 | 需手动配置（MaxIdleConns 等） | go-redis 内置连接池，默认即可 |
| 失败处理 | `log.Fatal` 退出 | `log.Fatal` 退出 |

两者设计思路一致：启动时初始化 → 全局单例 → 失败即 Fatal → 后续业务通过 `database.DB` / `database.RDB` 访问。

#### 2. 组织标签模型

更新：创建 `internal/model/org_tag.go`

核心目的是用树形结构来表示组织层级，从而实现多租户的文档权限隔离.
- 文档的归属与访问控制 —— 每个文档（file_upload）关联一个 org_tag，搜索时根据用户所属的组织标签来过滤可见文档。
- 组织的层级结构 —— 通过 ParentTag 实现树形组织架构（如：公司 → 部门 → 小组），子组织的成员可以向上继承父组织的文档访问权限。

