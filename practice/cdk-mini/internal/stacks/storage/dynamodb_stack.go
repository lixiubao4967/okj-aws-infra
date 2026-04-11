package storage

import (
	"fmt"

	customConstructs "cdk-mini/internal/constructs"

	awsconstructsv10 "github.com/aws/constructs-go/constructs/v10"

	"github.com/aws/aws-cdk-go/awscdk/v2"
)

// DynamoDBTableConfig 是 DynamoDB Stack 级别的配置
type DynamoDBTableConfig struct {
	TableName    string
	PartitionKey string
	AutoCleanup  bool
}

// DynamoDBStackProps 嵌入 awscdk.StackProps
type DynamoDBStackProps struct {
	awscdk.StackProps
	// TasksTable 是本项目的任务记录表（可按需扩展更多表）
	TasksTable *DynamoDBTableConfig
}

// DynamoDBStack 包含 DynamoDB 表的 CloudFormation Stack
// 注意：DynamoDB 与 S3 分开独立成 Stack，而非合并到一起
// 原因：两者的更新频率不同，S3 桶名一旦确定极少变动，而 DynamoDB 索引/容量模式可能调整
//       独立 Stack 让你可以只 deploy DynamoDB Stack 而不影响 S3 配置，减少回滚风险
type DynamoDBStack struct {
	awscdk.Stack
	TasksTable *customConstructs.DynamoDBTableConstruct
}

func NewDynamoDBStack(
	scope awsconstructsv10.Construct,
	id string,
	props *DynamoDBStackProps,
) *DynamoDBStack {
	var stack DynamoDBStack
	stack.Stack = awscdk.NewStack(scope, &id, &props.StackProps)

	if props.TasksTable != nil {
		stack.createTasksTable(props.TasksTable)
	}

	return &stack
}

func (s *DynamoDBStack) createTasksTable(config *DynamoDBTableConfig) {
	table, err := customConstructs.NewDynamoDBTableConstruct(
		s.Stack, config.TableName,
		&customConstructs.DynamoDBTableConstructProps{
			TableName:    config.TableName,
			PartitionKey: config.PartitionKey,
			AutoCleanup:  config.AutoCleanup,
		},
	)
	if err != nil {
		panic(fmt.Sprintf("failed to create DynamoDB table %s: %v", config.TableName, err))
	}

	s.TasksTable = table
}

// ConstructTasksTableConfig 是配置工厂函数
// TODO：请你来实现这个函数
// 目标：返回一个 *DynamoDBTableConfig，固化以下约定：
//   - TableName 和 PartitionKey 来自参数
//   - AutoCleanup 在练习环境中应该是 true（方便 cdk destroy 清理资源）
//
// 对比思考：
//   - okj-cdk-exchange 中类似的工厂函数在 services/helper/aurora_helper.go
//   - 工厂函数的价值：调用方只需传入变化的参数，稳定的约定（如 AutoCleanup）集中管理
func ConstructTasksTableConfig(tableName, partitionKey string) *DynamoDBTableConfig {
	return &DynamoDBTableConfig{
		TableName:    tableName,
		PartitionKey: partitionKey,
		AutoCleanup:  false, // 保守策略：DynamoDB 数据不随 cdk destroy 自动删除
	}
}
