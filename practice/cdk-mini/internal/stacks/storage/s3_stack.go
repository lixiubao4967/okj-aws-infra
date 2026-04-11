package storage

import (
	"fmt"

	customConstructs "cdk-mini/internal/constructs"

	awsconstructsv10 "github.com/aws/constructs-go/constructs/v10"

	"github.com/aws/aws-cdk-go/awscdk/v2"
)

// S3BucketConfig 是 Stack 级别的配置（比 ConstructProps 更高层）
type S3BucketConfig struct {
	BucketName  string
	Versioned   bool
	AutoCleanup bool
}

// S3StackProps 嵌入 awscdk.StackProps（包含 Account/Region/Description）
type S3StackProps struct {
	awscdk.StackProps
	PracticeBucket *S3BucketConfig
}

// S3Stack 是存储层的 CloudFormation Stack
// 暴露 PracticeBucket 供其他 Stack 引用（如 IAM 授权）
type S3Stack struct {
	awscdk.Stack
	PracticeBucket *customConstructs.S3BucketConstruct
}

func NewS3Stack(
	scope awsconstructsv10.Construct,
	id string,
	props *S3StackProps,
) *S3Stack {
	var stack S3Stack
	stack.Stack = awscdk.NewStack(scope, &id, &props.StackProps)

	if props.PracticeBucket != nil {
		stack.createPracticeBucket(props.PracticeBucket)
	}

	return &stack
}

func (s *S3Stack) createPracticeBucket(config *S3BucketConfig) {
	bucket, err := customConstructs.NewS3BucketConstruct(
		s.Stack, config.BucketName,
		&customConstructs.S3BucketConstructProps{
			BucketName:  config.BucketName,
			Versioned:   config.Versioned,
			AutoCleanup: config.AutoCleanup,
		},
	)
	if err != nil {
		// 项目约定：构造失败在 cdk synth 阶段暴露，不允许 silent failure
		panic(fmt.Sprintf("failed to create S3 bucket %s: %v", config.BucketName, err))
	}

	s.PracticeBucket = bucket
}

// ConstructPracticeBucketConfig 是配置工厂函数
// 固化项目约定（如练习环境总是开启 AutoCleanup），调用方只传动态参数
func ConstructPracticeBucketConfig(bucketName string) *S3BucketConfig {
	return &S3BucketConfig{
		BucketName:  bucketName,
		Versioned:   false,    // 练习环境不需要版本控制
		AutoCleanup: true,     // cdk destroy 时自动清空
	}
}
