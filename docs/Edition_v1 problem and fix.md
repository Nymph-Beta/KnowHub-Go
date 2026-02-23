## 之前架构的问题

当前架构：

```
AdminService（大杂烩）
 ├── orgTagRepo    → 组织标签 CRUD
 ├── userRepo     → 用户列表、分配标签
 └── conversationRepo → 对话审计（重复造轮子）

ConversationService（已存在但被忽视）
 └── conversationRepo → 对话读写
```

三个问题：

1. AdminService 违反单一职责：一个接口里混了三个不相关的业务域

1. 对话逻辑重复：AdminService.GetAllConversations 直接操作 conversationRepo，绕过了已有的 ConversationService

1. AdminHandler 注入了不需要的 UserService：仅仅为了在 CreateOrganizationTag 中调 GetProfile 获取创建者信息

---

已经仔细看完了 v2 的全部代码。你的重构质量很高，相比 v1 有很多实质性改进。先总结现状，再给出架构建议。

---

## v2 现状分析

**已实现的改进（做得好的地方）：**

| 改进              | v1                                          | v2                                                           |
| ----------------- | ------------------------------------------- | ------------------------------------------------------------ |
| 错误处理          | 返回裸 `errors.New`，Handler 逐个判断字符串 | 哨兵错误 + `mapServiceError` 统一映射                        |
| 删除标签          | 无子标签检查，靠数据库 `ON DELETE SET NULL` | `Delete`（保护删除）+ `DeleteAndReparentChildren`（重挂删除），都有事务 |
| Repository 健壮性 | 无参数校验                                  | nil 检查、空 ID 检查、`RowsAffected` 检查                    |
| Update 安全性     | `db.Save` 全字段覆盖                        | `Select` 限定字段，避免零值覆盖                              |
| API 参数          | `SetPrimaryOrg(username, orgTag)`           | `SetPrimaryOrg(userID, orgTagID)`，用稳定标识符              |
| ProfileResponse   | 定义了但没用                                | 实际使用，OrgTags 转为 `[]string`                            |
| 测试              | 无                                          | Service 和 Handler 层都有 fake mock 测试                     |
| BFS 遍历方向      | 向上遍历祖先                                | 向下遍历子节点（语义不同，后面讨论）                         |

**待实现：** `admin_service.go` 为空，管理员功能尚未开始。

---

## 推荐架构

基于你已有的代码结构，最合适的下一步是**按业务域拆分，不创建 AdminService**。

### 整体结构

```
internal/
├── handler/
│   ├── user_handler.go          ← 已有（普通用户操作）
│   ├── org_tag_handler.go       ← 新建（标签管理，挂在 admin 路由下）
│   └── helper.go                ← 提取公共函数（mapServiceError, extractBearerToken 等）
├── service/
│   ├── user_service.go          ← 已有，扩展 ListUsers + AssignOrgTagsToUser
│   ├── org_tag_service.go       ← 新建（标签域的业务逻辑）
│   └── admin_service.go         ← 删除（不需要）
├── repository/
│   ├── user_repository.go       ← 已有
│   └── org_tag_repository.go    ← 已有
├── middleware/
│   ├── auth.go                  ← 已有
│   └── admin_auth.go            ← 已有
└── model/
    ├── user.go                  ← 已有
    └── org_tag.go               ← 已有
```

### 1. 新建 `OrgTagService`

标签自身的业务逻辑从 v1 的 `AdminService` 中独立出来：

```go
// internal/service/org_tag_service.go

type OrgTagService interface {
    Create(tagID, name, description string, parentTag *string, createdBy string) (*model.OrganizationTag, error)
    Update(tagID, name, description string, parentTag *string, updatedBy string) (*model.OrganizationTag, error)
    Delete(tagID string) error                          // 保护删除
    DeleteAndReparent(tagID string) error               // 重挂删除
    List() ([]model.OrganizationTag, error)
    GetTree() ([]*model.OrganizationTagNode, error)     // 构建树形结构
    FindByID(tagID string) (*model.OrganizationTag, error)
}

type orgTagService struct {
    orgTagRepo repository.OrganizationTagRepository
}
```

为什么需要这一层而不直接在 Handler 里调 Repository？

- `Create` 需要**检查 TagID 是否重复**（v1 有这个逻辑）
- `GetTree` 需要**内存中构建树**（纯数据转换逻辑，不属于 Repository）
- `Delete` 可以添加**关联检查**（是否有用户或文档引用该标签）
- 统一的哨兵错误转换（`gorm.ErrRecordNotFound` → `ErrOrgTagNotFound`）

### 2. 扩展 `UserService`

把 v1 `AdminService` 里的用户管理方法移到 `UserService`：

```go
// 在现有 UserService 接口中新增：
type UserService interface {
    // ... 已有的方法 ...

    // 以下为管理员可调用的用户管理方法（权限由 middleware 控制）
    ListUsers(page, size int) ([]UserDetailDTO, int64, error)
    AssignOrgTagsToUser(userID uint, orgTagIDs []string) error
    FindByID(userID uint) (*model.User, error)
}
```

这些本质上是"用户域"的操作，只是权限上限定管理员才能调用——权限控制交给 `AdminAuthMiddleware`，而不是靠放在 `AdminService` 里暗示。

**关于 UserService 依赖 OrgTagService 的问题**：你当前 `UserService` 直接持有 `orgTagRepo`，短期内可以保持不变。如果后续 `OrgTagService` 的逻辑变复杂（比如创建标签时需要发通知、记审计日志），再把依赖从 `orgTagRepo` 切换到 `orgTagService`。不必提前过度设计。

### 3. 新建 `OrgTagHandler`

```go
// internal/handler/org_tag_handler.go

type OrgTagHandler struct {
    orgTagService service.OrgTagService
}

func (h *OrgTagHandler) Create(c *gin.Context)      { ... }
func (h *OrgTagHandler) List(c *gin.Context)         { ... }
func (h *OrgTagHandler) GetTree(c *gin.Context)      { ... }
func (h *OrgTagHandler) Update(c *gin.Context)       { ... }
func (h *OrgTagHandler) Delete(c *gin.Context)       { ... }
```

### 4. 路由注册

```go
// cmd/server/main.go

// --- 已有：普通用户路由 ---
users := r.Group("/api/v1/users")
{
    users.POST("/register", userHandler.Register)
    users.POST("/login", userHandler.Login)

    authed := users.Group("/")
    authed.Use(middleware.AuthMiddleware(jwtManager, userService))
    {
        authed.GET("/me", userHandler.GetProfile)
        authed.POST("/logout", userHandler.Logout)
        authed.PUT("/primary-org", userHandler.SetPrimaryOrg)
        authed.GET("/org-tags", userHandler.GetUserOrgTags)
    }
}

// --- 新增：管理员路由 ---
admin := r.Group("/api/v1/admin")
admin.Use(middleware.AuthMiddleware(jwtManager, userService), middleware.AdminAuthMiddleware())
{
    // 用户管理（调用 UserHandler 的管理方法）
    admin.GET("/users", userHandler.ListUsers)
    admin.PUT("/users/:userId/org-tags", userHandler.AssignOrgTagsToUser)

    // 标签管理（调用 OrgTagHandler）
    orgTags := admin.Group("/org-tags")
    {
        orgTags.POST("", orgTagHandler.Create)
        orgTags.GET("", orgTagHandler.List)
        orgTags.GET("/tree", orgTagHandler.GetTree)
        orgTags.PUT("/:id", orgTagHandler.Update)
        orgTags.DELETE("/:id", orgTagHandler.Delete)
    }
}
```

注意：`userHandler.ListUsers` 和 `userHandler.AssignOrgTagsToUser` 放在 `UserHandler` 中，只是路由挂在 admin 组下受 `AdminAuthMiddleware` 保护。**同一个 Handler 的方法可以挂在不同的路由组下**，权限由中间件决定。

### 5. 依赖注入（main.go）

```go
// Repository
userRepo := repository.NewUserRepository(database.DB)
orgTagRepo := repository.NewOrganizationTagRepository(database.DB)

// Service
orgTagService := service.NewOrgTagService(orgTagRepo)
userService := service.NewUserService(userRepo, orgTagRepo, jwtManager)

// Handler
userHandler := handler.NewUserHandler(userService)
orgTagHandler := handler.NewOrgTagHandler(orgTagService)
```

### 依赖关系图

```
┌──────────────────────────────────────────────────────┐
│  Middleware                                            │
│  AuthMiddleware        → 所有认证用户                  │
│  AdminAuthMiddleware   → 仅管理员                     │
└──────────────────────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────┐
│  Handler 层                                           │
│                                                       │
│  UserHandler              OrgTagHandler               │
│  ├─ Register              ├─ Create                   │
│  ├─ Login                 ├─ List                     │
│  ├─ GetProfile            ├─ GetTree                  │
│  ├─ Logout                ├─ Update                   │
│  ├─ SetPrimaryOrg         └─ Delete                   │
│  ├─ GetUserOrgTags                                    │
│  ├─ ListUsers      ← admin路由                        │
│  └─ AssignOrgTags  ← admin路由                        │
└──────────────────────────────────────────────────────┘
           │                       │
           ▼                       ▼
┌──────────────────────────────────────────────────────┐
│  Service 层                                           │
│                                                       │
│  UserService              OrgTagService               │
│  ├─ userRepo              └─ orgTagRepo               │
│  └─ orgTagRepo                                        │
│                                                       │
│  ※ 无 AdminService，权限由 Middleware 控制             │
└──────────────────────────────────────────────────────┘
           │                       │
           ▼                       ▼
┌──────────────────────────────────────────────────────┐
│  Repository 层                                        │
│                                                       │
│  UserRepository       OrganizationTagRepository       │
│                       ├─ Delete (保护)                │
│                       └─ DeleteAndReparent (重挂)     │
└──────────────────────────────────────────────────────┘
```

---

## 一个需要注意的设计差异

你的 v2 `GetUserEffectiveOrgTags` 是**向下遍历子节点**（BFS children），而 v1 是**向上遍历父节点**（BFS parents）。这两种方向的语义完全不同：

| 方向    | 语义                                         | 示例                         |
| ------- | -------------------------------------------- | ---------------------------- |
| v1 向上 | 用户属于"AI组"，也自动属于"技术部"和"公司"   | 能看到所有祖先组织的公开文档 |
| v2 向下 | 用户属于"技术部"，也自动拥有"AI组"和"后端组" | 能看到所有下级组织的文档     |

取决于你的业务需求，确认一下这个方向是否是你想要的。如果管理员被分配了顶层标签，向下遍历意味着他能看到所有下级的文档，这通常是合理的。但如果普通员工被分配了叶子节点，他就看不到上级组织的公开文档了。大多数企业系统会**两个方向都遍历**，或者根据具体业务选择一个。

---

## 实施顺序建议

1. **提取公共函数** — 把 `mapServiceError`、`extractBearerToken`、`parseOrgTagIDsForResponse` 从 `user_handler.go` 提到 `handler/helper.go`
2. **创建 `OrgTagService`** — 从 Repository 上面加一层薄业务逻辑（重复检查、树构建、哨兵错误转换）
3. **创建 `OrgTagHandler`** — CRUD + Tree 的 HTTP 翻译层
4. **扩展 `UserService`** — 添加 `ListUsers` 和 `AssignOrgTagsToUser`
5. **扩展 `UserHandler`** — 添加对应的 Handler 方法
6. **注册路由** — admin 路由组，挂上 `AdminAuthMiddleware`
7. **删除空的 `admin_service.go`**
8. **补充测试** — 为新增的 Service 和 Handler 方法写测试