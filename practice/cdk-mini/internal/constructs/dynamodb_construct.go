package constructs

import (
	"errors"

	awsconstructsv10 "github.com/aws/constructs-go/constructs/v10"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/jsii-runtime-go"
)

// DynamoDBTableConstructProps 封装 DynamoDB 表的配置
type DynamoDBTableConstructProps struct {
	TableName    string
	PartitionKey string // 分区键属性名
	// AutoCleanup：cdk destroy 时是否自动删除表（仅练习环境）
	AutoCleanup bool
}

// DynamoDBTableConstruct 封装单个 DynamoDB 表
type DynamoDBTableConstruct struct {
	awsconstructsv10.Construct
	table awsdynamodb.Table
}

func NewDynamoDBTableConstruct(
	scope awsconstructsv10.Construct,
	id string,
	props *DynamoDBTableConstructProps,
) (*DynamoDBTableConstruct, error) {
	if err := validateDynamoDBProps(props); err != nil {
		return nil, err
	}

	var c DynamoDBTableConstruct
	c.Construct = awsconstructsv10.NewConstruct(scope, &id)

	tableProps := &awsdynamodb.TableProps{
		TableName: jsii.String(props.TableName),
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String(props.PartitionKey),
			Type: awsdynamodb.AttributeType_STRING,
		},
		// PAY_PER_REQUEST：按需计费，练习时不会产生预留容量费用
		BillingMode: awsdynamodb.BillingMode_PAY_PER_REQUEST,
	}

	if props.AutoCleanup {
		tableProps.RemovalPolicy = awscdk.RemovalPolicy_DESTROY
	}

	c.table = awsdynamodb.NewTable(c.Construct, jsii.String("Table"), tableProps)

	return &c, nil
}

func (c *DynamoDBTableConstruct) GetTable() awsdynamodb.Table {
	return c.table
}

func (c *DynamoDBTableConstruct) GetTableName() *string {
	return c.table.TableName()
}

func (c *DynamoDBTableConstruct) GetTableArn() *string {
	return c.table.TableArn()
}

func validateDynamoDBProps(props *DynamoDBTableConstructProps) error {
	if props == nil {
		return errors.New("DynamoDBTableConstructProps cannot be nil")
	}

	if props.TableName == "" {
		return errors.New("TableName is required")
	}

	if props.PartitionKey == "" {
		return errors.New("PartitionKey is required")
	}

	return nil
}
