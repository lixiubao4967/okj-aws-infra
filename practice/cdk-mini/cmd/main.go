package main

import (
	"cdk-mini/internal/config"
	"cdk-mini/internal/stacks/messaging"
	"cdk-mini/internal/stacks/storage"
	"log"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	awsconstructsv10 "github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	// 1. 读取 CDK context 中的 env 参数（默认 "personal"）
	//    命令行覆盖：cdk synth --context env=personal
	env := getEnvironment(app)

	// 2. 从 config/{env}.yaml 加载配置（缺失/非法配置在这一步直接失败）
	envConfig, err := config.LoadConfig(env)
	if err != nil {
		log.Fatalf("failed to load config for env=%s: %v", env, err)
	}

	// 3. 构造 AWS 环境（账号 + 区域），所有 Stack 复用这个值
	awsEnv := &awscdk.Environment{
		Account: jsii.String(envConfig.Account),
		Region:  jsii.String(envConfig.Region),
	}

	// ──────────────────────────────────────────
	// 创建顺序 = 依赖顺序（和 okj-cdk-exchange 的 main.go 一致）
	// ──────────────────────────────────────────

	// Step 1：存储层（S3 + DynamoDB）—— 其他资源可能需要读写这里的资源
	s3Stack := setupS3Stack(app, env, awsEnv, envConfig)
	dynamoStack := setupDynamoDBStack(app, env, awsEnv, envConfig)

	// Step 2：消息层（SNS）—— 依赖存储层的 ARN（未来扩展）
	_ = setupSNSStack(app, env, awsEnv, envConfig)

	// 防止 lint 报 "declared and not used"（真实项目会把这些 Stack 放进 okjProps）
	_ = s3Stack
	_ = dynamoStack

	// 生成 CloudFormation 模板（cdk synth 实际执行这一行）
	app.Synth(nil)
}

// getEnvironment 从 CDK context 读取 env 参数，默认值 "personal"
// 对应 okj-cdk-exchange 中同名函数的简化版
func getEnvironment(app awscdk.App) string {
	raw := app.Node().TryGetContext(jsii.String("env"))
	if raw == nil {
		return "personal"
	}

	if env, ok := raw.(string); ok && env != "" {
		return env
	}

	return "personal"
}

// ─────────────────────────────────────────────────────────────────
// 各 Stack 的工厂函数（对应 okj-cdk-exchange main.go 中的 Setup* 函数）
// ─────────────────────────────────────────────────────────────────

func setupS3Stack(
	app awsconstructsv10.Construct,
	env string,
	awsEnv *awscdk.Environment,
	envConfig *config.EnvConfig,
) *storage.S3Stack {
	return storage.NewS3Stack(app, "cdk-mini-s3-"+env, &storage.S3StackProps{
		StackProps: awscdk.StackProps{
			Env:         awsEnv,
			Description: jsii.String("CDK Mini Practice — S3 storage layer"),
		},
		PracticeBucket: storage.ConstructPracticeBucketConfig(envConfig.S3.BucketName),
	})
}

func setupDynamoDBStack(
	app awsconstructsv10.Construct,
	env string,
	awsEnv *awscdk.Environment,
	envConfig *config.EnvConfig,
) *storage.DynamoDBStack {
	return storage.NewDynamoDBStack(app, "cdk-mini-dynamo-"+env, &storage.DynamoDBStackProps{
		StackProps: awscdk.StackProps{
			Env:         awsEnv,
			Description: jsii.String("CDK Mini Practice — DynamoDB tables"),
		},
		// 注意：这里调用的是你需要实现的工厂函数
		TasksTable: storage.ConstructTasksTableConfig(
			envConfig.DynamoDB.TableName,
			envConfig.DynamoDB.PartitionKey,
		),
	})
}

func setupSNSStack(
	app awsconstructsv10.Construct,
	env string,
	awsEnv *awscdk.Environment,
	envConfig *config.EnvConfig,
) *messaging.SNSStack {
	return messaging.NewSNSStack(app, "cdk-mini-sns-"+env, &messaging.SNSStackProps{
		StackProps: awscdk.StackProps{
			Env:         awsEnv,
			Description: jsii.String("CDK Mini Practice — SNS messaging layer"),
		},
		NotificationTopic: messaging.ConstructNotificationTopicConfig(
			envConfig.SNS.TopicName,
			envConfig.SNS.DisplayName,
		),
	})
}
