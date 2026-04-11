package messaging

import (
	"fmt"

	customConstructs "cdk-mini/internal/constructs"

	awsconstructsv10 "github.com/aws/constructs-go/constructs/v10"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"
)

// SNSTopicConfig 是 Stack 级别的 SNS 配置
type SNSTopicConfig struct {
	TopicName   string
	DisplayName string
}

// SNSStackProps 嵌入 awscdk.StackProps
type SNSStackProps struct {
	awscdk.StackProps
	NotificationTopic *SNSTopicConfig
}

// SNSStack 包含 SNS Topic 的 CloudFormation Stack
type SNSStack struct {
	awscdk.Stack
	NotificationTopic *customConstructs.SNSTopicConstruct
}

func NewSNSStack(
	scope awsconstructsv10.Construct,
	id string,
	props *SNSStackProps,
) *SNSStack {
	var stack SNSStack
	stack.Stack = awscdk.NewStack(scope, &id, &props.StackProps)

	if props.NotificationTopic != nil {
		stack.createNotificationTopic(props.NotificationTopic)
	}

	return &stack
}

func (s *SNSStack) createNotificationTopic(config *SNSTopicConfig) {
	var displayName *string
	if config.DisplayName != "" {
		displayName = jsii.String(config.DisplayName)
	}

	topic, err := customConstructs.NewSNSTopicConstruct(
		s.Stack, config.TopicName,
		&customConstructs.SNSTopicConstructProps{
			TopicName:   config.TopicName,
			DisplayName: displayName,
		},
	)
	if err != nil {
		panic(fmt.Sprintf("failed to create SNS topic %s: %v", config.TopicName, err))
	}

	s.NotificationTopic = topic
}

// ConstructNotificationTopicConfig 配置工厂函数
func ConstructNotificationTopicConfig(topicName, displayName string) *SNSTopicConfig {
	return &SNSTopicConfig{
		TopicName:   topicName,
		DisplayName: displayName,
	}
}
