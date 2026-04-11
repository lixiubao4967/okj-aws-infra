package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// EnvConfig 对应 config/{env}.yaml 的完整结构
type EnvConfig struct {
	Account  string    `yaml:"account"`
	Region   string    `yaml:"region"`
	S3       S3Config  `yaml:"s3"`
	SNS      SNSConfig `yaml:"sns"`
	DynamoDB DynamoDB  `yaml:"dynamodb"`
}

type S3Config struct {
	BucketName string `yaml:"bucketName"`
}

type SNSConfig struct {
	TopicName   string `yaml:"topicName"`
	DisplayName string `yaml:"displayName"`
}

type DynamoDB struct {
	TableName    string `yaml:"tableName"`
	PartitionKey string `yaml:"partitionKey"`
}

// LoadConfig 从 config/{env}.yaml 加载配置
func LoadConfig(env string) (*EnvConfig, error) {
	configPath := resolveConfigPath(env)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file %s: %w", configPath, err)
	}

	var cfg EnvConfig
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse config file %s: %w", configPath, err)
	}

	if err = validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config for env=%s: %w", env, err)
	}

	return &cfg, nil
}

// resolveConfigPath 找到 config/{env}.yaml 的绝对路径
// CDK 运行时总是从项目根目录（cdk.json 所在目录）执行，因此使用相对路径即可
func resolveConfigPath(env string) string {
	return filepath.Join("config", env+".yaml")
}

func validateConfig(cfg *EnvConfig) error {
	if cfg.Account == "" || cfg.Account == "YOUR_AWS_ACCOUNT_ID" {
		return errors.New("account is required — please update config/personal.yaml with your AWS account ID")
	}

	if cfg.Region == "" {
		return errors.New("region is required")
	}

	if cfg.S3.BucketName == "" {
		return errors.New("s3.bucketName is required")
	}

	if cfg.SNS.TopicName == "" {
		return errors.New("sns.topicName is required")
	}

	if cfg.DynamoDB.TableName == "" {
		return errors.New("dynamodb.tableName is required")
	}

	if cfg.DynamoDB.PartitionKey == "" {
		return errors.New("dynamodb.partitionKey is required")
	}

	return nil
}
