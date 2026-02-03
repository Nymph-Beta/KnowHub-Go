package main

import (
	"fmt"
	"pai_smart_go_v2/internal/config"
	"pai_smart_go_v2/pkg/log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"moul.io/zapgorm2"
)

// ============================================================================
// 模型定义
// ============================================================================

// DemoUser 用户模型 - 用于 GORM 功能演示
// GORM 会根据结构体字段自动映射数据库表结构
type DemoUser struct {
	ID        uint           `gorm:"primaryKey"`           // 主键，自增
	Name      string         `gorm:"size:100;not null"`    // 姓名，非空，最大100字符
	Email     string         `gorm:"uniqueIndex;size:200"` // 邮箱，唯一索引
	Age       int            `gorm:"default:0"`            // 年龄，默认值0
	CreatedAt time.Time      // GORM 自动管理：创建时自动设置
	UpdatedAt time.Time      // GORM 自动管理：更新时自动设置
	DeletedAt gorm.DeletedAt `gorm:"index"` // 软删除：删除时设置时间戳而非真正删除
}

// TableName 指定表名，避免与现有 users 表冲突
func (DemoUser) TableName() string {
	return "demo_users"
}

// ============================================================================
// 数据库初始化
// ============================================================================
//
// 阅读说明：
// 1. 打开连接：使用 gorm.Open(driver, &gorm.Config{...}) 建立与 MySQL 的连接。
//    第一个参数是驱动（如 mysql.Open(dsn)），第二个参数是 GORM 的配置。
//
// 2. gorm.Config 常用配置：
//    - Logger: 日志器，可控制 SQL 输出（Silent/Error/Warn/Info）。
//    - NamingStrategy: 表名/列名命名策略（如蛇形、自定义前缀）。
//    - NowFunc: 自定义当前时间（如 CreatedAt/UpdatedAt 的取值）。
//    - PrepareStmt: 是否缓存预编译语句，可提升重复查询性能。
//    - DisableForeignKeyConstraintWhenMigrating: 迁移时是否禁用外键约束。
//    - SkipDefaultTransaction: 是否关闭默认事务（按需优化性能）。
//
// 3. 为什么要配置连接池？
//    连接池复用已建立的 TCP 连接，避免每次请求都建连/断连，降低延迟并控制
//    总连接数，保护数据库。生产环境应显式设置；本 demo 为演示用，使用默认池即可。
//
// 4. 连接池三参数（本 demo 未设置，使用 Go 默认值）：
//    - SetMaxOpenConns(n): 允许打开的最大连接数，与 DB 的 max_connections 相关。
//    - SetMaxIdleConns(n): 空闲连接池中保留的最大连接数，过多占资源，过少会频繁建连。
//    - SetConnMaxLifetime(d): 单条连接最大存活时间，超时后回收，避免被服务端断开。
//    本文件是演示脚本、短生命周期、低并发，故不显式配置；正式服务建议在初始化时设置。
//

// initDemoDB 初始化数据库连接并执行迁移

// ============================================================================
// 数据库初始化
// ============================================================================

// initDemoDB 初始化数据库连接并执行迁移
func initDemoDB() (*gorm.DB, error) {
	dsn := config.Conf.Database.MySQL.DSN

	// 使用你导出的 zapLogger 创建 GORM logger
	gormLogger := zapgorm2.New(log.GetLogger())
	gormLogger.SetAsDefault() // 可选：设为默认

	// 1. 打开连接：gorm.Open(驱动, 配置)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	// 自动迁移：根据结构体创建/更新表结构
	if err := db.AutoMigrate(&DemoUser{}); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	return db, nil
}

// ============================================================================
// CRUD 演示函数
// ============================================================================

// demoCreate 演示 Create 操作
func demoCreate(db *gorm.DB) {
	printSection("Create 创建")

	// 1. 创建单条记录
	user := DemoUser{
		Name:  "张三",
		Email: "zhangsan@demo.com",
		Age:   25,
	}
	result := db.Create(&user)
	if result.Error != nil {
		fmt.Printf("  [错误] 创建失败: %v\n", result.Error)
		return
	}
	fmt.Printf("  [单条创建] ID: %d, Name: %s, 影响行数: %d\n", user.ID, user.Name, result.RowsAffected)

	// 2. 批量创建
	users := []DemoUser{
		{Name: "李四", Email: "lisi@demo.com", Age: 30},
		{Name: "王五", Email: "wangwu@demo.com", Age: 28},
		{Name: "赵六", Email: "zhaoliu@demo.com", Age: 22},
	}
	result = db.Create(&users)
	if result.Error != nil {
		fmt.Printf("  [错误] 批量创建失败: %v\n", result.Error)
		return
	}
	fmt.Printf("  [批量创建] 成功创建 %d 条记录\n", result.RowsAffected)

	// 3. 使用 map 创建（不依赖结构体）
	db.Model(&DemoUser{}).Create(map[string]interface{}{
		"Name":  "钱七",
		"Email": "qianqi@demo.com",
		"Age":   35,
	})
	fmt.Println("  [Map创建] 使用 map 创建记录成功")
}

// demoRead 演示 Read 查询操作
func demoRead(db *gorm.DB) {
	printSection("Read 查询")

	// 1. 根据主键查询
	var user DemoUser
	db.First(&user, 1) // 等价于 SELECT * FROM demo_users WHERE id = 1
	fmt.Printf("  [主键查询] First(&user, 1): ID=%d, Name=%s\n", user.ID, user.Name)

	// 2. 条件查询 - Where
	var userByEmail DemoUser
	db.Where("email = ?", "lisi@demo.com").First(&userByEmail)
	fmt.Printf("  [条件查询] Where email: Name=%s, Age=%d\n", userByEmail.Name, userByEmail.Age)

	// 3. 查询多条
	var allUsers []DemoUser
	db.Find(&allUsers)
	fmt.Printf("  [查询全部] Find: 共 %d 条记录\n", len(allUsers))

	// 4. 条件 + 排序 + 分页
	var sortedUsers []DemoUser
	db.Where("age > ?", 20).Order("age DESC").Limit(3).Offset(0).Find(&sortedUsers)
	fmt.Printf("  [组合查询] age>20, 按年龄降序, 取前3条:\n")
	for _, u := range sortedUsers {
		fmt.Printf("    - %s, %d岁\n", u.Name, u.Age)
	}

	// 5. 选择特定字段
	var names []string
	db.Model(&DemoUser{}).Pluck("name", &names)
	fmt.Printf("  [Pluck] 获取所有姓名: %v\n", names)

	// 6. 统计
	var count int64
	db.Model(&DemoUser{}).Where("age >= ?", 25).Count(&count)
	fmt.Printf("  [Count] 年龄>=25的用户数: %d\n", count)
}

// demoUpdate 演示 Update 更新操作
func demoUpdate(db *gorm.DB) {
	printSection("Update 更新")

	// 先查询一条记录用于演示
	var user DemoUser
	db.Where("email = ?", "zhangsan@demo.com").First(&user)

	// 1. 更新单个字段 - Update
	db.Model(&user).Update("Age", 26)
	fmt.Printf("  [单字段更新] Update: %s 的年龄更新为 26\n", user.Name)

	// 2. 更新多个字段 - Updates (结构体方式，零值会被忽略)
	db.Model(&user).Updates(DemoUser{Name: "张三丰", Age: 27})
	fmt.Printf("  [结构体更新] Updates: Name=%s, Age=%d\n", user.Name, user.Age)

	// 3. 更新多个字段 - Updates (map方式，可以更新为零值)
	db.Model(&user).Updates(map[string]interface{}{"Age": 0})
	fmt.Println("  [Map更新] 使用 map 可以将 Age 更新为 0")

	// 4. 批量更新
	result := db.Model(&DemoUser{}).Where("age < ?", 25).Update("age", 25)
	fmt.Printf("  [批量更新] 将 age<25 的记录更新为 25，影响 %d 行\n", result.RowsAffected)

	// 5. 使用表达式更新
	db.Model(&DemoUser{}).Where("email = ?", "lisi@demo.com").
		Update("age", gorm.Expr("age + ?", 1))
	fmt.Println("  [表达式更新] 李四的年龄 +1")
}

// demoDelete 演示 Delete 删除操作
func demoDelete(db *gorm.DB) {
	printSection("Delete 删除")

	// 1. 软删除（默认行为，设置 DeletedAt）
	var user DemoUser
	db.Where("email = ?", "qianqi@demo.com").First(&user)
	db.Delete(&user)
	fmt.Printf("  [软删除] Delete: %s 已被软删除\n", user.Name)

	// 2. 软删除后的查询行为
	var activeUsers []DemoUser
	db.Find(&activeUsers)
	fmt.Printf("  [查询验证] 软删除后，Find 只返回 %d 条活跃记录\n", len(activeUsers))

	// 3. 包含软删除记录的查询
	var allUsers []DemoUser
	db.Unscoped().Find(&allUsers)
	fmt.Printf("  [Unscoped] 包含软删除，共 %d 条记录\n", len(allUsers))

	// 4. 恢复软删除的记录
	db.Unscoped().Model(&user).Update("deleted_at", nil)
	fmt.Printf("  [恢复] %s 的软删除已恢复\n", user.Name)

	// 5. 永久删除（物理删除）
	db.Unscoped().Delete(&user)
	fmt.Printf("  [永久删除] Unscoped().Delete: %s 已被永久删除\n", user.Name)
}

// demoAdvanced 演示高级功能
func demoAdvanced(db *gorm.DB) {
	printSection("高级功能")

	// 1. 事务 - 自动管理
	fmt.Println("  [事务-自动]")
	err := db.Transaction(func(tx *gorm.DB) error {
		// 在事务中创建用户
		if err := tx.Create(&DemoUser{
			Name:  "事务用户A",
			Email: "tx_a@demo.com",
			Age:   20,
		}).Error; err != nil {
			return err // 返回错误会自动回滚
		}

		if err := tx.Create(&DemoUser{
			Name:  "事务用户B",
			Email: "tx_b@demo.com",
			Age:   21,
		}).Error; err != nil {
			return err
		}

		fmt.Println("    事务内创建 2 个用户成功")
		return nil // 返回 nil 提交事务
	})
	if err != nil {
		fmt.Printf("    事务失败并回滚: %v\n", err)
	} else {
		fmt.Println("    事务提交成功")
	}

	// 2. 原生 SQL
	fmt.Println("  [原生SQL]")
	var count int64
	db.Raw("SELECT COUNT(*) FROM demo_users WHERE deleted_at IS NULL").Scan(&count)
	fmt.Printf("    Raw SQL 查询活跃用户数: %d\n", count)

	// 3. 执行原生 SQL 修改
	db.Exec("UPDATE demo_users SET age = age + 1 WHERE email LIKE ?", "%@demo.com")
	fmt.Println("    Exec SQL 批量更新年龄 +1")

	// 4. FirstOrCreate - 查找或创建
	fmt.Println("  [FirstOrCreate]")
	var newUser DemoUser
	db.Where(DemoUser{Email: "test_foc@demo.com"}).
		Attrs(DemoUser{Name: "FirstOrCreate用户", Age: 18}). // 如果创建，使用这些属性
		FirstOrCreate(&newUser)
	fmt.Printf("    FirstOrCreate: ID=%d, Name=%s (已存在则查询，不存在则创建)\n", newUser.ID, newUser.Name)

	// 5. Scopes - 可复用的查询条件
	fmt.Println("  [Scopes]")
	adultScope := func(db *gorm.DB) *gorm.DB {
		return db.Where("age >= ?", 18)
	}
	var adults []DemoUser
	db.Scopes(adultScope).Find(&adults)
	fmt.Printf("    使用 Scope 查询成年用户: %d 人\n", len(adults))
}

// ============================================================================
// 辅助函数
// ============================================================================

// printSection 打印分隔线和标题
func printSection(title string) {
	fmt.Printf("\n────────────────────────────────────────\n")
	fmt.Printf("  %s\n", title)
	fmt.Printf("────────────────────────────────────────\n")
}

// cleanupDemo 清理所有测试数据
func cleanupDemo(db *gorm.DB) {
	printSection("清理测试数据")

	// 永久删除所有 demo 数据
	result := db.Unscoped().Where("email LIKE ?", "%@demo.com").Delete(&DemoUser{})
	fmt.Printf("  已永久删除 %d 条测试数据\n", result.RowsAffected)
}

// ============================================================================
// 主入口
// ============================================================================

// GormDemo GORM 功能演示主函数
// 演示内容：连接数据库、CRUD 操作、事务、原生 SQL 等
func GormDemo() {
	fmt.Println("\n╔════════════════════════════════════════╗")
	fmt.Println("║         GORM 功能演示                  ║")
	fmt.Println("╚════════════════════════════════════════╝")

	// 1. 初始化数据库
	db, err := initDemoDB()
	if err != nil {
		fmt.Printf("[错误] %v\n", err)
		return
	}
	fmt.Println("\n✓ 数据库连接成功")
	fmt.Println("✓ 表迁移完成 (demo_users)")

	// 2. 清理旧的测试数据（确保每次运行环境干净）
	db.Unscoped().Where("email LIKE ?", "%@demo.com").Delete(&DemoUser{})

	// 3. 执行各功能演示
	demoCreate(db)   // 创建
	demoRead(db)     // 查询
	demoUpdate(db)   // 更新
	demoDelete(db)   // 删除
	demoAdvanced(db) // 高级功能

	// 4. 清理测试数据
	cleanupDemo(db)

	fmt.Println("\n╔════════════════════════════════════════╗")
	fmt.Println("║         演示结束                       ║")
	fmt.Println("╚════════════════════════════════════════╝\n")
}
