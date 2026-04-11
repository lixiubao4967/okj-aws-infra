package props

import (
	"cdk-mini/internal/config"
	"cdk-mini/internal/stacks/messaging"
	"cdk-mini/internal/stacks/storage"

	awsconstructsv10 "github.com/aws/constructs-go/constructs/v10"
)

// MiniStackProps 是 okj-cdk-exchange OkjStackProps 的精简版
// 把所有已创建的 Stack 汇聚在一起，方便跨 Stack 引用
// （例如：若后续添加 Lambda，可以从这里取到 S3Stack 进行授权）
type MiniStackProps struct {
	App          awsconstructsv10.Construct
	EnvConfig    *config.EnvConfig
	S3Stack      *storage.S3Stack
	DynamoStack  *storage.DynamoDBStack
	MessagingStack *messaging.SNSStack
}
