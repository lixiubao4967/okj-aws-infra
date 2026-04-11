package constructs

import (
	"errors"

	awsconstructsv10 "github.com/aws/constructs-go/constructs/v10"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/jsii-runtime-go"
)

// S3BucketConstructProps 封装 S3 桶的配置
type S3BucketConstructProps struct {
	BucketName string
	// Versioned：是否启用版本控制（练习时设 false，生产环境推荐 true）
	Versioned bool
	// AutoCleanup：删除 Stack 时自动清空桶内文件（仅限练习/测试环境）
	AutoCleanup bool
}

// S3BucketConstruct 封装单个 S3 桶
type S3BucketConstruct struct {
	awsconstructsv10.Construct
	bucket awss3.Bucket
}

func NewS3BucketConstruct(
	scope awsconstructsv10.Construct,
	id string,
	props *S3BucketConstructProps,
) (*S3BucketConstruct, error) {
	if err := validateS3BucketProps(props); err != nil {
		return nil, err
	}

	var c S3BucketConstruct
	c.Construct = awsconstructsv10.NewConstruct(scope, &id)

	bucketProps := &awss3.BucketProps{
		BucketName:        jsii.String(props.BucketName),
		Versioned:         jsii.Bool(props.Versioned),
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
	}

	// 练习环境：允许 cdk destroy 自动清空并删除桶
	if props.AutoCleanup {
		bucketProps.AutoDeleteObjects = jsii.Bool(true)
		bucketProps.RemovalPolicy = awscdk.RemovalPolicy_DESTROY
	}

	c.bucket = awss3.NewBucket(c.Construct, jsii.String("Bucket"), bucketProps)

	return &c, nil
}

func (c *S3BucketConstruct) GetBucket() awss3.Bucket {
	return c.bucket
}

func (c *S3BucketConstruct) GetBucketName() *string {
	return c.bucket.BucketName()
}

func (c *S3BucketConstruct) GetBucketArn() *string {
	return c.bucket.BucketArn()
}

func validateS3BucketProps(props *S3BucketConstructProps) error {
	if props == nil {
		return errors.New("S3BucketConstructProps cannot be nil")
	}

	if props.BucketName == "" {
		return errors.New("BucketName is required")
	}

	return nil
}
