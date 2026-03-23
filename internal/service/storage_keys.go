package service

import "fmt"

func buildUploadObjectKey(userID uint, fileMD5 string, fileName string) string {
	return fmt.Sprintf("uploads/%d/%s/%s", userID, fileMD5, fileName)
}

func buildChunkObjectKey(fileMD5 string, chunkIndex int) string {
	return fmt.Sprintf("chunks/%s/%d", fileMD5, chunkIndex)
}
