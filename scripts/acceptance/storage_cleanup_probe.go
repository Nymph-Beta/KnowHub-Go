package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type result struct {
	RedisKey         string `json:"redisKey"`
	RedisExists      bool   `json:"redisExists"`
	MinioPrefix      string `json:"minioPrefix"`
	MinioObjectCount int    `json:"minioObjectCount"`
	WaitedMs         int64  `json:"waitedMs"`
}

func countObjects(ctx context.Context, client *minio.Client, bucket, prefix string) (int, error) {
	count := 0
	opts := minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}
	for obj := range client.ListObjects(ctx, bucket, opts) {
		if obj.Err != nil {
			return 0, obj.Err
		}
		count++
	}
	return count, nil
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func main() {
	redisAddr := flag.String("redis-addr", "127.0.0.1:6379", "Redis address")
	redisPassword := flag.String("redis-password", "", "Redis password")
	redisDB := flag.Int("redis-db", 1, "Redis DB index")
	redisKey := flag.String("redis-key", "", "Redis key to check")

	minioEndpoint := flag.String("minio-endpoint", "127.0.0.1:9300", "MinIO API endpoint")
	minioAccessKey := flag.String("minio-access-key", "minioadmin", "MinIO access key")
	minioSecretKey := flag.String("minio-secret-key", "minioadmin", "MinIO secret key")
	minioUseSSL := flag.Bool("minio-use-ssl", false, "MinIO use SSL")
	minioBucket := flag.String("minio-bucket", "uploads", "MinIO bucket")
	minioPrefix := flag.String("minio-prefix", "", "MinIO object prefix to check")

	timeoutSec := flag.Int("timeout-sec", 20, "Wait timeout (seconds)")
	pollMS := flag.Int("poll-ms", 500, "Poll interval (milliseconds)")
	flag.Parse()

	if *redisKey == "" || *minioPrefix == "" {
		fmt.Fprintln(os.Stderr, "redis-key and minio-prefix are required")
		os.Exit(1)
	}

	ctx := context.Background()

	rdb := redis.NewClient(&redis.Options{
		Addr:     *redisAddr,
		Password: *redisPassword,
		DB:       *redisDB,
	})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Fprintf(os.Stderr, "redis ping failed: %v\n", err)
		os.Exit(1)
	}

	minioClient, err := minio.New(*minioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(*minioAccessKey, *minioSecretKey, ""),
		Secure: *minioUseSSL,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "minio client init failed: %v\n", err)
		os.Exit(1)
	}

	start := time.Now()
	timeout := time.Duration(*timeoutSec) * time.Second
	interval := time.Duration(*pollMS) * time.Millisecond
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}

	last := result{
		RedisKey:    *redisKey,
		MinioPrefix: *minioPrefix,
	}

	for {
		exists, err := rdb.Exists(ctx, *redisKey).Result()
		if err != nil {
			fmt.Fprintf(os.Stderr, "redis EXISTS failed: %v\n", err)
			os.Exit(1)
		}
		last.RedisExists = exists > 0

		count, err := countObjects(ctx, minioClient, *minioBucket, *minioPrefix)
		if err != nil {
			fmt.Fprintf(os.Stderr, "minio list objects failed: %v\n", err)
			os.Exit(1)
		}
		last.MinioObjectCount = count
		last.WaitedMs = time.Since(start).Milliseconds()

		if !last.RedisExists && last.MinioObjectCount == 0 {
			printJSON(last)
			os.Exit(0)
		}

		if time.Since(start) >= timeout {
			printJSON(last)
			os.Exit(2)
		}

		time.Sleep(interval)
	}
}
