package tasks

// FileProcessingTask 是 Kafka 中用于触发文档处理管线的消息体。
// objectKey 由生产端直接提供，消费者不再拼接存储路径规则。
type FileProcessingTask struct {
	FileMD5   string `json:"file_md5"`
	FileName  string `json:"file_name"`
	UserID    uint   `json:"user_id"`
	OrgTag    string `json:"org_tag"`
	IsPublic  bool   `json:"is_public"`
	ObjectKey string `json:"object_key"`
}
