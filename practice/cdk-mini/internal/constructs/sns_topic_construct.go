package constructs

import (
	"errors"

	awsconstructsv10 "github.com/aws/constructs-go/constructs/v10"

	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/jsii-runtime-go"
)

// SNSTopicConstructProps 封装 SNS Topic 所需的全部参数
// 指针类型字段 = 可选（nil 时跳过）
type SNSTopicConstructProps struct {
	TopicName   string
	DisplayName *string // 可选：控制台显示名
}

// SNSTopicConstruct 是对单个 SNS Topic 的封装
// 职责：校验参数、创建资源、提供 getter
type SNSTopicConstruct struct {
	awsconstructsv10.Construct
	topic awssns.Topic
}

// NewSNSTopicConstruct 遵循项目约定：(scope, id, props) → (*T, error)
func NewSNSTopicConstruct(
	scope awsconstructsv10.Construct,
	id string,
	props *SNSTopicConstructProps,
) (*SNSTopicConstruct, error) {
	if err := validateSNSTopicProps(props); err != nil {
		return nil, err
	}

	var c SNSTopicConstruct
	c.Construct = awsconstructsv10.NewConstruct(scope, &id)

	c.topic = awssns.NewTopic(c.Construct, jsii.String("Topic"), &awssns.TopicProps{
		TopicName:   jsii.String(props.TopicName),
		DisplayName: props.DisplayName,
	})

	return &c, nil
}

func (c *SNSTopicConstruct) GetTopic() awssns.Topic {
	return c.topic
}

func (c *SNSTopicConstruct) GetTopicArn() *string {
	return c.topic.TopicArn()
}

func validateSNSTopicProps(props *SNSTopicConstructProps) error {
	if props == nil {
		return errors.New("SNSTopicConstructProps cannot be nil")
	}

	if props.TopicName == "" {
		return errors.New("TopicName is required")
	}

	return nil
}
