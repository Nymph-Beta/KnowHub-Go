// 负责对象存储服务的初始化、上传、下载等交互操作
package storage

import (
	"context"
	"pai_smart_go_v2/internal/config"
	"pai_smart_go_v2/pkg/log"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var MinIOClient *minio.Client

// 初始化 MinIO 客户端
func InitMinio(cfg config.MinIOConfig) {
	var err error

	// 创建 MinIO 客户端
	MinIOClient, err = minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		log.Fatal("Failed to create MinIO client", err)
		return
	}

	log.Info("MinIO client created successfully")

	// 2. 检查桶是否存在，不存在则创建
	ctx := context.Background()
	bucketName := cfg.BucketName

	exists, err := MinIOClient.BucketExists(ctx, bucketName)
	if err != nil {
		log.Fatal("Failed to check bucket existence", err)
		return
	}
	if !exists {
		err = MinIOClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			log.Fatal("Failed to create bucket", err)
			return
		}
		log.Infof("Bucket %s created successfully", bucketName)
	} else {
		log.Infof("Bucket %s already exists", bucketName)
	}
}
